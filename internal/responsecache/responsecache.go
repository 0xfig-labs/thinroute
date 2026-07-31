package responsecache

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/config"
	"github.com/0xfig-labs/thinroute/internal/cache"
	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/usage"
)

const responseCachePrefix = "gomodel:response:"

// Cache type constants used in response headers and audit/usage logging.
const (
	CacheTypeExact   = "exact"
	CacheHeaderExact = "HIT (exact)"
)

var internalRequestHeaderAllowlist = map[string]struct{}{
	http.CanonicalHeaderKey("Accept"):                     {},
	http.CanonicalHeaderKey("Baggage"):                    {},
	http.CanonicalHeaderKey("Cache-Control"):              {},
	http.CanonicalHeaderKey("Content-Type"):               {},
	http.CanonicalHeaderKey("Request-Id"):                 {},
	http.CanonicalHeaderKey("Traceparent"):                {},
	http.CanonicalHeaderKey("Tracestate"):                 {},
	http.CanonicalHeaderKey("User-Agent"):                 {},
	http.CanonicalHeaderKey("X-Cache-Control"):            {},
	http.CanonicalHeaderKey("X-Cache-Semantic-Threshold"): {},
	http.CanonicalHeaderKey("X-Cache-TTL"):                {},
	http.CanonicalHeaderKey("X-Cache-Type"):               {},
	http.CanonicalHeaderKey("X-Request-ID"):               {},
}

// ResponseCacheMiddleware wraps response cache logic. App and server only see this type.
type ResponseCacheMiddleware struct {
	simple *simpleCacheMiddleware
}

// InternalHandleResult is the buffered result of running the cache middleware
// for a transport-free internal JSON request.
type InternalHandleResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	CacheType  string
}

// NewResponseCacheMiddleware creates middleware from config.
// Only simple/exact cache is supported.
func NewResponseCacheMiddleware(
	cfg config.ResponseCacheConfig,
	resolvedProviders map[string]config.RawProviderConfig,
	usageLogger usage.LoggerInterface,
	pricingResolver usage.PricingResolver,
) (*ResponseCacheMiddleware, error) {
	m := &ResponseCacheMiddleware{}
	hitRecorder := newUsageHitRecorder(usageLogger, pricingResolver)

	if cfg.Simple != nil && config.SimpleCacheEnabled(cfg.Simple) {
		ttl := time.Duration(cfg.Simple.TTL) * time.Second
		if ttl == 0 {
			ttl = time.Hour
		}
		store := cache.NewMapStore()
		m.simple = newSimpleCacheMiddleware(store, ttl, hitRecorder)
		slog.Info("response cache (simple/exact) enabled", "ttl_seconds", int(ttl.Seconds()), "backend", "memory")
	} else if cfg.Simple != nil {
		slog.Info("response cache (simple/exact) disabled by config")
	}

	return m, nil
}

// HandleRequest runs the cache check for a translated inference request.
// Only exact/simple cache is supported.
func (m *ResponseCacheMiddleware) HandleRequest(c *echo.Context, body []byte, next func() error) error {
	if m == nil {
		return next()
	}
	return m.handle(&echoExchange{c: c}, body, next)
}

// handle runs the cache check against any transport.
func (m *ResponseCacheMiddleware) handle(ex exchange, body []byte, next func() error) error {
	if shouldSkipAllCacheHeaders(ex.RequestHeader) {
		return next()
	}

	if m.simple != nil {
		hit, err := m.simple.TryHit(ex, body)
		if err != nil || hit {
			return err
		}
		return m.simple.StoreAfter(ex, body, next)
	}

	return next()
}

// HandleInternalRequest runs the cache for a transport-free internal JSON
// request. Request headers are derived from the originating request snapshot
// (allowlisted), and next executes the LLM call, returning a buffered
// response instead of writing to a socket.
func (m *ResponseCacheMiddleware) HandleInternalRequest(
	ctx context.Context,
	method, path string,
	body []byte,
	next func(ctx context.Context) (*InternalResponse, error),
) (*InternalHandleResult, error) {
	if ctx == nil {
		return nil, core.NewInvalidRequestError("context is required", nil)
	}
	if m == nil {
		slog.Error("response cache: HandleInternalRequest called on nil middleware")
		return nil, core.NewProviderError("", http.StatusInternalServerError, "response cache middleware is not initialized", nil)
	}

	ex := newInternalExchange(ctx, method, path, next)
	err := m.handle(ex, body, ex.runNext)
	if err != nil {
		var gatewayErr *core.GatewayError
		if errors.As(err, &gatewayErr) && gatewayErr != nil {
			return nil, gatewayErr
		}
		return nil, core.NewProviderError("", http.StatusInternalServerError, err.Error(), err)
	}

	return ex.result(), nil
}

// Close waits for any in-flight cache writes to complete, then releases cache resources.
func (m *ResponseCacheMiddleware) Close() error {
	if m == nil {
		return nil
	}
	if m.simple != nil {
		return m.simple.close()
	}
	return nil
}

func internalRequestHeaders(ctx context.Context) http.Header {
	headers := make(http.Header)
	if snapshot := core.GetRequestSnapshot(ctx); snapshot != nil {
		for key, values := range snapshot.HeadersView() {
			key = http.CanonicalHeaderKey(key)
			if _, allowed := internalRequestHeaderAllowlist[key]; !allowed {
				continue
			}
			for _, value := range values {
				headers.Add(key, value)
			}
		}
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	if requestID := strings.TrimSpace(core.GetRequestID(ctx)); requestID != "" && headers.Get("X-Request-ID") == "" {
		headers.Set("X-Request-ID", requestID)
	}
	return headers
}

func internalCacheType(headerValue string) string {
	headerValue = strings.TrimSpace(headerValue)
	if strings.HasPrefix(headerValue, "HIT (") && strings.HasSuffix(headerValue, ")") {
		headerValue = strings.TrimSpace(headerValue[len("HIT (") : len(headerValue)-1])
	}
	switch headerValue {
	case CacheTypeExact:
		return CacheTypeExact
	default:
		return ""
	}
}

// NewResponseCacheMiddlewareWithStore creates middleware with a custom store (for testing).
func NewResponseCacheMiddlewareWithStore(store cache.Store, ttl time.Duration) *ResponseCacheMiddleware {
	return &ResponseCacheMiddleware{
		simple: newSimpleCacheMiddleware(store, ttl, nil),
	}
}

// newUsageHitRecorder creates a hit recorder that logs usage for cache hits.
// When usageLogger is nil, no recording takes place.
func newUsageHitRecorder(usageLogger usage.LoggerInterface, _ usage.PricingResolver) func(exchange, []byte, string) {
	if usageLogger == nil {
		return nil
	}
	return func(ex exchange, cached []byte, cacheType string) {
		usageLogger.Write(&usage.UsageEntry{
			Endpoint:  ex.Path(),
			CacheType: cacheType,
		})
	}
}

// shouldSkipAllCacheHeaders returns true when the request carries a header
// that explicitly disables cache participation.
func shouldSkipAllCacheHeaders(h func(string) string) bool {
	return strings.EqualFold(h("X-Cache-Control"), "no-cache") ||
		strings.EqualFold(h("Cache-Control"), "no-cache") ||
		strings.EqualFold(h("Cache-Control"), "no-store")
}
