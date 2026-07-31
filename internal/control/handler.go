// Package control provides the control REST API.
package control

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/providers"
	"github.com/icehugh/thinroute/internal/usage"
	"github.com/icehugh/thinroute/internal/virtualmodels"
)

// Handler serves control API endpoints.
type Handler struct {
	usageReader         usage.UsageReader
	registry            *providers.ModelRegistry
	virtualModels       *virtualmodels.Service
	runtimeConfig       RuntimeConfigResponse
	runtimeRefresher    RuntimeRefresher
	configuredProviders []providers.SanitizedProviderConfig
	cooldownTracker     *providers.CooldownTracker

	mutationMu sync.Mutex
}

// Option configures the control API handler.
type Option func(*Handler)

const (
	RuntimeConfigLoggingEnabled = "LOGGING_ENABLED"
	RuntimeConfigUsageEnabled   = "USAGE_ENABLED"
	RuntimeConfigCacheEnabled   = "CACHE_ENABLED"
)

// statusClientClosedRequest is the de facto status used by proxies for client-aborted requests.
const statusClientClosedRequest = 499

// RuntimeConfigResponse is the allowlisted runtime config contract exposed to the control clients.
type RuntimeConfigResponse struct {
	LoggingEnabled string `json:"LOGGING_ENABLED,omitempty"`
	UsageEnabled   string `json:"USAGE_ENABLED,omitempty"`
	CacheEnabled   string `json:"CACHE_ENABLED,omitempty"`
}

type providerStatusSummaryResponse struct {
	Total         int    `json:"total"`
	Healthy       int    `json:"healthy"`
	Degraded      int    `json:"degraded"`
	Unhealthy     int    `json:"unhealthy"`
	OverallStatus string `json:"overall_status"`
}

type providerStatusItemResponse struct {
	Name         string                            `json:"name"`
	Type         string                            `json:"type"`
	Status       string                            `json:"status"`
	StatusLabel  string                            `json:"status_label"`
	StatusReason string                            `json:"status_reason"`
	LastError    string                            `json:"last_error,omitempty"`
	Config       providers.SanitizedProviderConfig `json:"config"`
	Runtime      providers.ProviderRuntimeSnapshot `json:"runtime"`
}

type providerStatusResponse struct {
	Summary   providerStatusSummaryResponse `json:"summary"`
	Providers []providerStatusItemResponse  `json:"providers"`
}

const (
	RuntimeRefreshStatusOK      = "ok"
	RuntimeRefreshStatusPartial = "partial"
	RuntimeRefreshStatusFailed  = "failed"
	RuntimeRefreshStatusSkipped = "skipped"
)

// RuntimeRefreshStep describes the result of one manual runtime refresh step.
type RuntimeRefreshStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// RuntimeRefreshReport is returned by the manual runtime refresh endpoint.
type RuntimeRefreshReport struct {
	Status        string               `json:"status"`
	StartedAt     time.Time            `json:"started_at"`
	FinishedAt    time.Time            `json:"finished_at"`
	DurationMS    int64                `json:"duration_ms"`
	ModelCount    int                  `json:"model_count"`
	ProviderCount int                  `json:"provider_count"`
	Steps         []RuntimeRefreshStep `json:"steps"`
}

// RuntimeRefresher refreshes application runtime snapshots on demand.
type RuntimeRefresher interface {
	RefreshRuntime(ctx context.Context) (RuntimeRefreshReport, error)
}

// WithRuntimeConfig enables the allowlisted runtime config endpoint.
func WithRuntimeConfig(values RuntimeConfigResponse) Option {
	return func(h *Handler) {
		h.runtimeConfig = normalizeRuntimeConfig(values)
	}
}

// WithRuntimeRefresher enables manual runtime refresh from the control API.
func WithRuntimeRefresher(refresher RuntimeRefresher) Option {
	return func(h *Handler) {
		h.runtimeRefresher = refresher
	}
}

// WithConfiguredProviders enables the         control-safe provider inventory endpoint.
func WithConfiguredProviders(configs []providers.SanitizedProviderConfig) Option {
	return func(h *Handler) {
		h.configuredProviders = cloneConfiguredProviders(configs)
	}
}

// WithCooldownTracker enables the provider cooldown status endpoint.
func WithCooldownTracker(ct *providers.CooldownTracker) Option {
	return func(h *Handler) {
		h.cooldownTracker = ct
	}
}

// WithVirtualModels enables virtual model configuration from the control API.
func WithVirtualModels(service *virtualmodels.Service) Option {
	return func(h *Handler) {
		h.virtualModels = service
	}
}

// NewHandler creates a new control API handler.
// usageReader may be nil if usage tracking is not available.
func NewHandler(reader usage.UsageReader, registry *providers.ModelRegistry, options ...Option) *Handler {
	h := &Handler{
		usageReader:   reader,
		registry:      registry,
		runtimeConfig: RuntimeConfigResponse{},
	}

	for _, opt := range options {
		if opt != nil {
			opt(h)
		}
	}

	return h
}

func normalizeRuntimeConfig(values RuntimeConfigResponse) RuntimeConfigResponse {
	return RuntimeConfigResponse{
		LoggingEnabled: strings.TrimSpace(values.LoggingEnabled),
		UsageEnabled:   strings.TrimSpace(values.UsageEnabled),
		CacheEnabled:   strings.TrimSpace(values.CacheEnabled),
	}
}

func cloneRuntimeConfig(values RuntimeConfigResponse) RuntimeConfigResponse {
	return values
}

func cloneConfiguredProviders(configs []providers.SanitizedProviderConfig) []providers.SanitizedProviderConfig {
	if len(configs) == 0 {
		return nil
	}
	cloned := make([]providers.SanitizedProviderConfig, len(configs))
	for i := range configs {
		cloned[i] = configs[i]
		if len(configs[i].Models) > 0 {
			cloned[i].Models = append([]string(nil), configs[i].Models...)
		}
	}
	return cloned
}

var validIntervals = map[string]bool{
	"daily":   true,
	"weekly":  true,
	"monthly": true,
	"yearly":  true,
}

const (
	usageTimeZoneHeader  = "X-GoModel-Timezone"
	defaultControlTZ     = "UTC"
	defaultDateRangeDays = usage.DefaultDateRangeDays
	maxDateRangeDays     = usage.MaxDateRangeDays
)

var timeNow = time.Now

// parseUsageParams extracts UsageQueryParams from the request query string.
// Returns an error if date parameters are provided but malformed.
func parseUsageParams(c *echo.Context) (usage.UsageQueryParams, error) {
	params, err := parseDateRangeParams(c)
	if err != nil {
		return params, err
	}

	// Parse interval
	params.Interval = c.QueryParam("interval")
	if !validIntervals[params.Interval] {
		params.Interval = "daily"
	}
	params.CacheMode = c.QueryParam("cache_mode")
	params.Model = strings.TrimSpace(c.QueryParam("model"))
	params.Provider = strings.TrimSpace(c.QueryParam("provider"))
	params.Label = strings.TrimSpace(c.QueryParam("label"))

	userPath, err := normalizeUserPathQueryParam("user_path", c.QueryParam("user_path"))
	if err != nil {
		return params, err
	}
	params.UserPath = userPath

	return params, nil
}

func normalizeUserPathQueryParam(fieldName, raw string) (string, error) {
	userPath, err := core.NormalizeUserPath(raw)
	if err != nil {
		return "", core.NewInvalidRequestError("invalid "+fieldName+": "+err.Error(), err)
	}
	return userPath, nil
}

// parseDateRangeParams extracts common date range query params.
// Returns an error if date parameters are provided but malformed.
func parseDateRangeParams(c *echo.Context) (usage.UsageQueryParams, error) {
	var params usage.UsageQueryParams

	timeZone, location := usageTimeZone(c)
	params.TimeZone = timeZone

	now := timeNow().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)

	days := defaultDateRangeDays
	if d := c.QueryParam("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = min(parsed, maxDateRangeDays)
		}
	}

	start, end, err := usage.BuildDateRange(strings.TrimSpace(c.QueryParam("start_date")), strings.TrimSpace(c.QueryParam("end_date")), days, location, today)
	if err != nil {
		return params, err
	}
	params.StartDate = start
	params.EndDate = end
	return params, nil
}

func usageTimeZone(c *echo.Context) (string, *time.Location) {
	value := strings.TrimSpace(c.Request().Header.Get(usageTimeZoneHeader))
	if value == "" {
		return defaultControlTZ, time.UTC
	}

	location, err := time.LoadLocation(value)
	if err != nil {
		return defaultControlTZ, time.UTC
	}

	return location.String(), location
}

// handleError converts errors to appropriate HTTP responses, matching the
// format used by the main API handlers in the server package.
func handleError(c *echo.Context, err error) error {
	if gatewayErr, ok := errors.AsType[*core.GatewayError](err); ok {
		logHandledAdminError(c, gatewayErr)
		return c.JSON(gatewayErr.HTTPStatusCode(), gatewayErr.ToJSON())
	}

	if errors.Is(err, context.Canceled) {
		gatewayErr := core.NewInvalidRequestErrorWithStatus(statusClientClosedRequest, "request canceled", err).
			WithCode("request_canceled")
		logHandledAdminError(c, gatewayErr)
		return c.JSON(gatewayErr.HTTPStatusCode(), gatewayErr.ToJSON())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		gatewayErr := core.NewInvalidRequestErrorWithStatus(http.StatusGatewayTimeout, "request timed out", err).
			WithCode("request_timeout")
		logHandledAdminError(c, gatewayErr)
		return c.JSON(gatewayErr.HTTPStatusCode(), gatewayErr.ToJSON())
	}

	fallback := &core.GatewayError{
		Type:       "internal_error",
		Message:    "an unexpected error occurred",
		StatusCode: http.StatusInternalServerError,
		Err:        err,
	}
	logHandledAdminError(c, fallback)
	return c.JSON(fallback.HTTPStatusCode(), fallback.ToJSON())
}

func logHandledAdminError(c *echo.Context, gatewayErr *core.GatewayError) {
	if gatewayErr == nil {
		return
	}

	attrs := []any{
		"type", gatewayErr.Type,
		"status", gatewayErr.HTTPStatusCode(),
		"message", gatewayErr.Message,
	}
	if gatewayErr.Provider != "" {
		attrs = append(attrs, "provider", gatewayErr.Provider)
	}
	if gatewayErr.Param != nil {
		attrs = append(attrs, "param", *gatewayErr.Param)
	}
	if gatewayErr.Code != nil {
		attrs = append(attrs, "code", *gatewayErr.Code)
	}
	if gatewayErr.Err != nil {
		attrs = append(attrs, "error", gatewayErr.Err)
	}
	if c != nil && c.Request() != nil {
		req := c.Request()
		attrs = append(attrs,
			"method", req.Method,
			"path", req.URL.Path,
		)
		if requestID := requestIDFromAdminContextOrHeader(req); requestID != "" {
			attrs = append(attrs, "request_id", requestID)
		}
	}

	status := gatewayErr.HTTPStatusCode()
	if status == statusClientClosedRequest {
		slog.Debug("control request canceled", attrs...)
		return
	}
	if status >= http.StatusInternalServerError {
		slog.Error("control request failed", attrs...)
		return
	}
	slog.Warn("control request failed", attrs...)
}

func requestIDFromAdminContextOrHeader(req *http.Request) string {
	if req == nil {
		return ""
	}
	if requestID := strings.TrimSpace(core.GetRequestID(req.Context())); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(req.Header.Get("X-Request-ID"))
}
