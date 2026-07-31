// Package app provides the main application struct for centralized dependency management
// and lifecycle control of the thinroute server.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/icehugh/thinroute/config"
	"github.com/icehugh/thinroute/ext"
	"github.com/icehugh/thinroute/internal/auditlog"
	"github.com/icehugh/thinroute/internal/batch"
	"github.com/icehugh/thinroute/internal/control"
	"github.com/icehugh/thinroute/internal/conversationstore"
	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/filestore"
	"github.com/icehugh/thinroute/internal/httpclient"
	"github.com/icehugh/thinroute/internal/providers"
	"github.com/icehugh/thinroute/internal/responsecache"
	"github.com/icehugh/thinroute/internal/responsestore"
	"github.com/icehugh/thinroute/internal/server"
	"github.com/icehugh/thinroute/internal/storage"
	"github.com/icehugh/thinroute/internal/usage"
	"github.com/icehugh/thinroute/internal/virtualmodels"
)

// App represents the main application with all its dependencies.
// It provides centralized lifecycle management for all components.
type App struct {
	config        *config.Config
	providers     *providers.InitResult
	audit         *auditlog.Result
	usage         *usage.Result
	batch         *batch.Result
	fileStore     *filestore.Result
	responseStore *responsestore.Result
	conversations *conversationstore.Result
	virtualModels *virtualmodels.Result
	server        *server.Server
	control       *server.ControlServer

	shutdownMu  sync.Mutex
	shutdown    bool
	serverMu    sync.Mutex
	serverStop  context.CancelFunc
	serverDone  chan error
	refreshCh   chan struct{}
	refreshOnce sync.Once
}

// Config holds the configuration options for creating an App.
type Config struct {
	// AppConfig holds the loaded application configuration and raw provider data
	// produced by config.Load.
	AppConfig *config.LoadResult

	// Factory provides the ProviderFactory used to construct provider instances.
	Factory *providers.ProviderFactory

	// Extensions optionally carries registered gateway extensions (request
	// rewriters, middleware, routes). The registry is snapshotted here; later
	// registrations have no effect.
	Extensions *ext.Registry
}

// applyExtensions snapshots a registered extension set into the server
// configuration. A nil registry leaves the config untouched.
func applyExtensions(serverCfg *server.Config, extensions *ext.Registry) {
	if extensions == nil {
		return
	}
	serverCfg.RequestRewriters = extensions.Rewriters()
	serverCfg.ExtraMiddleware = extensions.Middleware()
	serverCfg.ExtraRoutes = extensions.Routes()
	serverCfg.ExtraAuthSkipPaths = extensions.PublicPaths()
}

// New creates a new App with all dependencies initialized.
// The caller must call Shutdown to release resources.
func New(ctx context.Context, cfg Config) (*App, error) {
	if cfg.AppConfig == nil {
		return nil, fmt.Errorf("app config is required")
	}

	if cfg.AppConfig.Config == nil {
		return nil, fmt.Errorf("app config contains nil Config")
	}

	if cfg.Factory == nil {
		return nil, fmt.Errorf("factory is required")
	}

	appCfg := cfg.AppConfig.Config
	// Install config-file HTTP timeouts before any provider constructs a
	// transport; env vars still take precedence inside httpclient.
	httpclient.SetConfiguredTimeouts(appCfg.HTTP.Timeout, appCfg.HTTP.ResponseHeaderTimeout)

	app := &App{
		config: appCfg,
	}
	// closers collects the Close functions of successfully initialized
	// components; fail unwinds them in reverse order before returning an
	// initialization error.
	closers := []func() error{}
	fail := func(msg string, cause error) (*App, error) {
		var closeErrs []error
		for i := len(closers) - 1; i >= 0; i-- {
			closeErrs = append(closeErrs, closers[i]())
		}
		closeErr := errors.Join(closeErrs...)
		switch {
		case cause != nil && closeErr != nil:
			return nil, fmt.Errorf("%s: %w (also: close error: %v)", msg, cause, closeErr)
		case cause != nil:
			return nil, fmt.Errorf("%s: %w", msg, cause)
		case closeErr != nil:
			return nil, fmt.Errorf("%s (also: close error: %v)", msg, closeErr)
		default:
			return nil, errors.New(msg)
		}
	}

	// sharedStorage is the first non-nil storage backend among initialized
	// components; later stores reuse it instead of opening their own.
	var sharedStorage storage.Storage
	claimSharedStorage := func(s storage.Storage) {
		if sharedStorage == nil {
			sharedStorage = s
		}
	}

	providerResult, err := providers.Init(ctx, cfg.AppConfig, cfg.Factory)
	if err != nil {
		return fail("failed to initialize providers", err)
	}
	app.providers = providerResult
	closers = append(closers, app.providers.Close)

	// Initialize audit logging
	auditResult, err := auditlog.New(ctx, appCfg)
	if err != nil {
		return fail("failed to initialize audit logging", err)
	}
	app.audit = auditResult
	closers = append(closers, app.audit.Close)

	// Initialize usage tracking
	var usageResult *usage.Result
	usageResult, err = usage.New(ctx, appCfg)
	if err != nil {
		return fail("failed to initialize usage tracking", err)
	}
	if usageResult == nil || usageResult.Logger == nil {
		if usageResult != nil {
			closers = append(closers, usageResult.Close)
		}
		return fail("usage tracking initialization returned nil result", nil)
	}
	app.usage = usageResult
	closers = append(closers, app.usage.Close)
	claimSharedStorage(usageResult.Storage)

	// Initialize batch lifecycle storage.
	var batchResult *batch.Result
	if sharedStorage != nil {
		batchResult, err = batch.NewWithSharedStorage(ctx, sharedStorage)
	} else {
		batchResult, err = batch.New(ctx, appCfg)
	}
	if err != nil {
		return fail("failed to initialize batch storage", err)
	}
	app.batch = batchResult
	closers = append(closers, app.batch.Close)
	claimSharedStorage(batchResult.Storage)

	// Initialize file provider mapping storage for OpenAI-compatible Files/Batches.
	var fileStoreResult *filestore.Result
	if sharedStorage != nil {
		fileStoreResult, err = filestore.NewWithSharedStorage(ctx, sharedStorage)
	} else {
		fileStoreResult, err = filestore.New(ctx, appCfg)
	}
	if err != nil {
		return fail("failed to initialize file mapping storage", err)
	}
	app.fileStore = fileStoreResult
	closers = append(closers, app.fileStore.Close)
	claimSharedStorage(fileStoreResult.Storage)

	// Initialize Responses/Conversations lifecycle persistence so agentic
	// response chains and conversation history land in storage instead of
	// accumulating in process memory.
	var responseStoreResult *responsestore.Result
	if sharedStorage != nil {
		responseStoreResult, err = responsestore.NewWithSharedStorage(ctx, sharedStorage)
	} else {
		responseStoreResult, err = responsestore.New(ctx, appCfg)
	}
	if err != nil {
		return fail("failed to initialize response snapshot storage", err)
	}
	app.responseStore = responseStoreResult
	closers = append(closers, app.responseStore.Close)
	claimSharedStorage(responseStoreResult.Storage)

	var conversationStoreResult *conversationstore.Result
	if sharedStorage != nil {
		conversationStoreResult, err = conversationstore.NewWithSharedStorage(ctx, sharedStorage)
	} else {
		conversationStoreResult, err = conversationstore.New(ctx, appCfg)
	}
	if err != nil {
		return fail("failed to initialize conversation storage", err)
	}
	app.conversations = conversationStoreResult
	closers = append(closers, app.conversations.Close)
	claimSharedStorage(conversationStoreResult.Storage)

	// Initialize virtual models (unified aliases + access overrides) using
	// shared storage when already available. Provider names declared in YAML —
	// including entries whose credentials did not resolve, which never register —
	// let validation tell a misspelled target provider (abort startup) from a
	// declared-but-inactive one (warn, target stays unavailable).
	declaredProviders := make([]string, 0, len(cfg.AppConfig.RawProviders))
	for name := range cfg.AppConfig.RawProviders {
		declaredProviders = append(declaredProviders, name)
	}
	virtualModelsResult, err := virtualmodels.New(ctx, appCfg, providerResult.Registry, declaredProviders)
	if err != nil {
		return fail("failed to initialize virtual models", err)
	}
	app.virtualModels = virtualModelsResult
	closers = append(closers, app.virtualModels.Close)

	// The unified virtual models service is the single engine: it serves model
	// resolution (redirects), access authorization (policies), and exposed-model
	// listing.
	vm := app.virtualModels.Service

	pricingResolver := usage.PricingResolver(providerResult.Registry)
	// Build runtime execution dependencies. Policy is passed explicitly into the
	// server; the live provider dependency remains the bare router.
	var provider core.RoutableProvider = app.providers.Router
	var translatedRequestPatcher server.TranslatedRequestPatcher
	var batchRequestPreparers []server.BatchRequestPreparer

	if vm != nil {
		// One combined preparer rewrites redirect sources and validates access,
		// replacing the previous two-preparer pipeline.
		batchRequestPreparers = append([]server.BatchRequestPreparer{
			virtualmodels.NewBatchPreparer(provider, vm),
		}, batchRequestPreparers...)
	}
	batchRequestPreparer := server.ComposeBatchRequestPreparers(providerAsNativeFileRouter(provider), batchRequestPreparers...)
	allowPassthroughV1Alias := appCfg.Server.AllowPassthroughV1Alias

	serverUsageLogger := usage.LoggerInterface(usageResult.Logger)

	// Usage read storage comes from the usage subsystem.
	usageReadStorage := usageResult.Storage
	var usageReader usage.UsageReader
	if usageReadStorage != nil {
		var readerErr error
		usageReader, readerErr = usage.NewReader(usageReadStorage)
		if readerErr != nil {
			slog.Warn("usage reader unavailable; usage endpoints will omit usage data", "error", readerErr)
			usageReader = nil
		}
	}

	serverCfg := &server.Config{
		BasePath:                        appCfg.Server.BasePath,
		MetricsEnabled:                  appCfg.Metrics.Enabled,
		MetricsEndpoint:                 appCfg.Metrics.Endpoint,
		BodySizeLimit:                   appCfg.Server.BodySizeLimit,
		PprofEnabled:                    appCfg.Server.PprofEnabled,
		AuditLogger:                     auditResult.Logger,
		UsageLogger:                     serverUsageLogger,
		PricingResolver:                 pricingResolver,
		ModelResolver:                   vm,
		ModelAuthorizer:                 vm,
		TranslatedRequestPatcher:        translatedRequestPatcher,
		BatchRequestPreparer:            batchRequestPreparer,
		ExposedModelLister:              vm,
		KeepOnlyAliasesAtModelsEndpoint: appCfg.Models.KeepOnlyAliasesAtModelsEndpoint,
		PassthroughSemanticEnrichers:    cfg.Factory.PassthroughSemanticEnrichers(),
		BatchStore:                      batchResult.Store,
		FileStore:                       fileStoreResult.Store,
		ResponseStore:                   responseStoreResult.Store,
		ConversationStore:               conversationStoreResult.Store,
		LogOnlyModelInteractions:        appCfg.Logging.OnlyModelInteractions,
		DisablePassthroughRoutes:        !appCfg.Server.EnablePassthroughRoutes,
		EnabledPassthroughProviders:     appCfg.Server.EnabledPassthroughProviders,
		RealtimeEnabled:                 appCfg.Server.RealtimeEnabled,
		DisableReasoningModels:          reasoningDisabledModels(appCfg),
		AllowPassthroughV1Alias:         &allowPassthroughV1Alias,
		UserPathHeader:                  appCfg.Server.UserPathHeader,
	}

	if usageReader != nil {
		serverCfg.UsageSummarizer = usageReader
	}

	applyExtensions(serverCfg, cfg.Extensions)

	// Wire the readiness storage probe. Storage is a required dependency, so a
	// failed ping makes /health/ready report not_ready (503). When no storage
	// backend is active, readiness simply collapses to liveness.
	if hc, ok := sharedStorage.(storage.HealthChecker); ok {
		serverCfg.StorageProbe = hc
	}

	// Initialize the local control API on its own listener.
	controlCfg := appCfg.Control
	usageEnabledForControl := usageResult.Logger.Config().Enabled

	controlHandler, controlErr := initControl(
		usageReader,
		providerResult.Registry,
		providerResult.ConfiguredProviders,
		vm,
		app,
		controlRuntimeConfig(appCfg, usageEnabledForControl),
	)
	if controlErr != nil {
		return fail("failed to initialize control API", controlErr)
	}
	app.control = server.NewControlServer(controlHandler, "")
	slog.Info("control API enabled", "listen", controlCfg.Listen, "path", "/control/v1")

	if appCfg.Server.PprofEnabled {
		slog.Info("pprof enabled", "path", config.JoinBasePath(appCfg.Server.BasePath, "/debug/pprof/"))
	}
	if appCfg.Server.EnablePassthroughRoutes {
		slog.Info("provider passthrough enabled", "path", config.JoinBasePath(appCfg.Server.BasePath, "/p/{provider}/{endpoint}"))
	} else {
		slog.Info("provider passthrough disabled")
	}

	rcm, err := responsecache.NewResponseCacheMiddleware(appCfg.Cache.Response, providerResult.CredentialResolvedProviders, usageResult.Logger, pricingResolver)
	if err != nil {
		return fail("failed to initialize response cache", err)
	}
	closers = append(closers, rcm.Close)
	serverCfg.ResponseCacheMiddleware = rcm

	app.logStartupInfo()
	app.server = server.New(provider, serverCfg)

	return app, nil
}

// Router returns the core.RoutableProvider for request routing.
func (a *App) Router() core.RoutableProvider {
	if a.providers == nil {
		return nil
	}
	return a.providers.Router
}

// AuditLogger returns the audit logger interface.
func (a *App) AuditLogger() auditlog.LoggerInterface {
	if a.audit == nil {
		return nil
	}
	return a.audit.Logger
}

// UsageLogger returns the usage logger interface.
func (a *App) UsageLogger() usage.LoggerInterface {
	if a.usage == nil {
		return nil
	}
	return a.usage.Logger
}

func providerAsNativeFileRouter(provider core.RoutableProvider) core.NativeFileRoutableProvider {
	if fileRouter, ok := provider.(core.NativeFileRoutableProvider); ok {
		return fileRouter
	}
	return nil
}

func reasoningDisabledModels(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	models := append([]string(nil), cfg.Models.DisableReasoning...)
	for _, virtualModel := range cfg.VirtualModels {
		if virtualModel.DisableReasoning {
			models = append(models, virtualModel.Source)
		}
	}
	return models
}

// Start starts the HTTP server on the given address.
// This is a blocking call that returns when the server stops.
func (a *App) Start(ctx context.Context, addr string) error {
	return a.startServer(ctx, addr, func(serverCtx context.Context) error {
		return a.server.Start(serverCtx, addr)
	})
}

// StartWithListener starts the HTTP server on a pre-bound listener.
// This is primarily useful for tests that need to reserve a loopback port
// before handing control to the server.
func (a *App) StartWithListener(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("listener is required")
	}
	return a.startServer(ctx, listener.Addr().String(), func(serverCtx context.Context) error {
		return a.server.StartWithListener(serverCtx, listener)
	})
}

func (a *App) startServer(ctx context.Context, address string, start func(context.Context) error) error {
	if a.server == nil {
		return fmt.Errorf("server is not initialized")
	}

	var controlListener net.Listener
	var err error
	if a.control != nil {
		controlListener, err = net.Listen("tcp", a.config.Control.Listen)
		if err != nil {
			return fmt.Errorf("control server listen %s: %w", a.config.Control.Listen, err)
		}
	}

	a.serverMu.Lock()
	if a.serverDone != nil {
		a.serverMu.Unlock()
		if controlListener != nil {
			_ = controlListener.Close()
		}
		return fmt.Errorf("server is already running")
	}
	serverCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	a.serverStop = cancel
	a.serverDone = done
	a.serverMu.Unlock()

	if a.control != nil {
		go func() {
			if err := a.control.StartWithListener(serverCtx, controlListener); err != nil &&
				!errors.Is(err, http.ErrServerClosed) && serverCtx.Err() == nil {
				slog.Error("control server failed", "error", err)
				cancel()
			}
		}()
	}
	slog.Info("starting server", "address", address)
	err = start(serverCtx)

	a.serverMu.Lock()
	if a.serverDone == done {
		done <- err
		close(done)
		a.serverDone = nil
		a.serverStop = nil
	}
	a.serverMu.Unlock()

	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			slog.Info("server stopped gracefully")
			return nil
		}
		return fmt.Errorf("server failed to start: %w", err)
	}
	return nil
}

// Shutdown gracefully tears down app components in dependency order.
// Order:
// 1. Cancel HTTP server context, close live streams, and wait for the server to stop.
// 2. Provider subsystem close (stops model refresh loop and cache resources).
// 3. Batch store close.
// 4. Usage logger close (flushes pending usage records).
// 5. Audit logger close (flushes pending audit records).
//
// Shutdown is idempotent and safe for repeated calls; after the first call, subsequent calls are no-ops.
// It attempts every close step, aggregates failures, and returns a joined error if any step fails.
func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownMu.Lock()
	if a.shutdown {
		a.shutdownMu.Unlock()
		return nil
	}
	a.shutdown = true
	a.shutdownMu.Unlock()

	slog.Info("shutting down application...")

	var errs []error

	// 1. Stop HTTP server first (stop accepting new requests)
	a.serverMu.Lock()
	serverStop := a.serverStop
	serverDone := a.serverDone
	a.serverMu.Unlock()
	if serverStop != nil {
		serverStop()
	}
	if serverDone != nil {
		select {
		case err := <-serverDone:
			a.serverMu.Lock()
			a.serverDone = nil
			a.serverStop = nil
			a.serverMu.Unlock()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("server shutdown error", "error", err)
				errs = append(errs, fmt.Errorf("server shutdown: %w", err))
			}
		case <-ctx.Done():
			slog.Error("server shutdown timed out", "error", ctx.Err())
			errs = append(errs, fmt.Errorf("server shutdown: %w", ctx.Err()))
		}
	}

	// 2. Release server-owned resources now that no requests are in flight
	// (drains response cache writes, closes response/conversation stores).
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			slog.Error("server resources close error", "error", err)
			errs = append(errs, fmt.Errorf("server resources close: %w", err))
		}
	}

	// 3. Close providers (stops model refresh and provider-owned resources)
	if a.providers != nil {
		if err := a.providers.Close(); err != nil {
			slog.Error("providers close error", "error", err)
			errs = append(errs, fmt.Errorf("providers close: %w", err))
		}
	}

	// 4. Close virtual models subsystem (aliases + access overrides).
	if a.virtualModels != nil {
		if err := a.virtualModels.Close(); err != nil {
			slog.Error("virtual models close error", "error", err)
			errs = append(errs, fmt.Errorf("virtual models close: %w", err))
		}
	}

	// 11. Close file mapping store.
	if a.fileStore != nil {
		if err := a.fileStore.Close(); err != nil {
			slog.Error("file mapping store close error", "error", err)
			errs = append(errs, fmt.Errorf("file store close: %w", err))
		}
	}

	// 12. Close batch store (flushes pending entries)
	if a.batch != nil {
		if err := a.batch.Close(); err != nil {
			slog.Error("batch store close error", "error", err)
			errs = append(errs, fmt.Errorf("batch close: %w", err))
		}
	}

	// 14. Close usage tracking (flushes pending entries)
	if a.usage != nil {
		if err := a.usage.Close(); err != nil {
			slog.Error("usage logger close error", "error", err)
			errs = append(errs, fmt.Errorf("usage close: %w", err))
		}
	}

	// 15. Close audit logging (flushes pending logs)
	if a.audit != nil {
		if err := a.audit.Close(); err != nil {
			slog.Error("audit logger close error", "error", err)
			errs = append(errs, fmt.Errorf("audit close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
	}

	slog.Info("application shutdown complete")
	return nil
}

// logStartupInfo logs the application configuration on startup.
func (a *App) logStartupInfo() {
	cfg := a.config

	slog.Info("authentication disabled (local loopback gateway)")

	// Metrics configuration
	if cfg.Metrics.Enabled {
		slog.Info("prometheus metrics enabled", "endpoint", cfg.Metrics.Endpoint)
	} else {
		slog.Info("prometheus metrics disabled")
	}

	// Storage configuration (shared by audit logging and usage tracking)
	slog.Info("storage configured", "type", cfg.Storage.Type)

	// Audit logging configuration
	if cfg.Logging.Enabled {
		slog.Info("audit logging enabled", "retention_days", cfg.Logging.RetentionDays)
	} else {
		slog.Info("audit logging disabled")
	}

	// Usage tracking configuration
	if cfg.Usage.Enabled {
		slog.Info("usage tracking enabled",
			"buffer_size", cfg.Usage.BufferSize,
			"flush_interval", cfg.Usage.FlushInterval,
			"retention_days", cfg.Usage.RetentionDays,
		)
	} else {
		slog.Info("usage tracking disabled")
	}

}

// initControl creates the control API handler.
func initControl(
	reader usage.UsageReader,
	registry *providers.ModelRegistry,
	configuredProviders []providers.SanitizedProviderConfig,
	virtualModelService *virtualmodels.Service,
	runtimeRefresher control.RuntimeRefresher,
	runtimeConfig control.RuntimeConfigResponse,
) (*control.Handler, error) {
	controlHandler := control.NewHandler(
		reader,
		registry,
		control.WithConfiguredProviders(configuredProviders),
		control.WithVirtualModels(virtualModelService),
		control.WithRuntimeRefresher(runtimeRefresher),
		control.WithRuntimeConfig(runtimeConfig),
	)
	return controlHandler, nil
}

func controlRuntimeConfig(cfg *config.Config, usageEnabled bool) control.RuntimeConfigResponse {
	return control.RuntimeConfigResponse{
		LoggingEnabled: dashboardEnabledValue(cfg != nil && cfg.Logging.Enabled),
		UsageEnabled:   dashboardEnabledValue(cfg != nil && cfg.Usage.Enabled),
		CacheEnabled:   dashboardEnabledValue(cacheAnalyticsConfigured(cfg, usageEnabled)),
	}
}

func cacheAnalyticsConfigured(cfg *config.Config, usageEnabled bool) bool {
	return cfg != nil && usageEnabled && responseCacheConfigured(cfg.Cache.Response)
}

func dashboardEnabledValue(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func runtimeWorkflowFeatureCaps(cfg *config.Config) core.WorkflowFeatures {
	if cfg == nil {
		return core.WorkflowFeatures{}
	}
	return core.WorkflowFeatures{
		Cache: responseCacheConfigured(cfg.Cache.Response),
		Audit: cfg.Logging.Enabled,
		Usage: cfg.Usage.Enabled,
	}
}

func responseCacheConfigured(cfg config.ResponseCacheConfig) bool {
	return simpleResponseCacheConfiguredFromResponse(cfg)
}

func simpleResponseCacheConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return simpleResponseCacheConfiguredFromResponse(cfg.Cache.Response)
}

func simpleResponseCacheConfiguredFromResponse(cfg config.ResponseCacheConfig) bool {
	return cfg.Simple != nil && config.SimpleCacheEnabled(cfg.Simple)
}
