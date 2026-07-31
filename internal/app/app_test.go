package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/config"
	"github.com/0xfig-labs/thinroute/ext"
	"github.com/0xfig-labs/thinroute/internal/control"
	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/providers"
	"github.com/0xfig-labs/thinroute/internal/server"
)

type runtimeRefreshMockProvider struct {
	models *core.ModelsResponse
	err    error
}

func (m *runtimeRefreshMockProvider) ChatCompletion(_ context.Context, _ *core.ChatRequest) (*core.ChatResponse, error) {
	return nil, nil
}

func (m *runtimeRefreshMockProvider) StreamChatCompletion(_ context.Context, _ *core.ChatRequest) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}

func (m *runtimeRefreshMockProvider) ListModels(_ context.Context) (*core.ModelsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.models, nil
}

func (m *runtimeRefreshMockProvider) Responses(_ context.Context, _ *core.ResponsesRequest) (*core.ResponsesResponse, error) {
	return nil, nil
}

func (m *runtimeRefreshMockProvider) StreamResponses(_ context.Context, _ *core.ResponsesRequest) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}

func (m *runtimeRefreshMockProvider) Embeddings(_ context.Context, _ *core.EmbeddingRequest) (*core.EmbeddingResponse, error) {
	return nil, core.NewInvalidRequestError("not supported", nil)
}

func TestRefreshRuntime_RefreshesModelListProvidersAndRegistryCache(t *testing.T) {
	registry := providers.NewModelRegistry()
	registry.RegisterProviderWithNameAndType(&runtimeRefreshMockProvider{
		models: &core.ModelsResponse{
			Object: "list",
			Data: []core.Model{
				{ID: "gpt-test", Object: "model", OwnedBy: "openai"},
			},
		},
	}, "openai", "openai")

	modelListServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": 1,
			"updated_at": "2026-04-11T00:00:00Z",
			"providers": {
				"openai": {
					"display_name": "OpenAI",
					"api_type": "openai",
					"supported_modes": ["chat"]
				}
			},
			"models": {
				"gpt-test": {
					"display_name": "GPT Test",
					"modes": ["chat"],
					"context_window": 128000
				}
			},
			"provider_models": {}
		}`))
	}))
	defer modelListServer.Close()

	app := &App{
		config: &config.Config{
			Cache: config.CacheConfig{
				Model: config.ModelCacheConfig{
					ModelList: config.ModelListConfig{URL: modelListServer.URL},
				},
			},
		},
		providers: &providers.InitResult{Registry: registry},
	}

	report, err := app.RefreshRuntime(context.Background())
	if err != nil {
		t.Fatalf("RefreshRuntime() error = %v", err)
	}
	if report.Status != control.RuntimeRefreshStatusOK {
		t.Fatalf("RefreshRuntime().Status = %q, want ok; steps=%+v", report.Status, report.Steps)
	}
	if report.ModelCount != 1 || report.ProviderCount != 1 {
		t.Fatalf("RefreshRuntime() counts = %d/%d, want 1/1", report.ModelCount, report.ProviderCount)
	}

	info := registry.GetModel("openai/gpt-test")
	if info == nil || info.Model.Metadata == nil {
		t.Fatal("expected refreshed provider model metadata")
	}
	if info.Model.Metadata.DisplayName != "GPT Test" {
		t.Fatalf("DisplayName = %q, want GPT Test", info.Model.Metadata.DisplayName)
	}
	if info.Model.Metadata.ContextWindow == nil || *info.Model.Metadata.ContextWindow != 128000 {
		t.Fatalf("ContextWindow = %v, want 128000", info.Model.Metadata.ContextWindow)
	}
}

func TestRefreshRuntime_SkipsDisabledVirtualModels(t *testing.T) {
	registry := providers.NewModelRegistry()
	registry.RegisterProviderWithNameAndType(&runtimeRefreshMockProvider{
		models: &core.ModelsResponse{
			Object: "list",
			Data: []core.Model{
				{ID: "gpt-test", Object: "model", OwnedBy: "openai"},
			},
		},
	}, "openai", "openai")

	// virtualModels is left nil so the virtual_models refresh step reports
	// skipped, which is what this test asserts.
	app := &App{
		config: &config.Config{},
		providers: &providers.InitResult{
			Registry: registry,
		},
	}

	report, err := app.RefreshRuntime(context.Background())
	if err != nil {
		t.Fatalf("RefreshRuntime() error = %v", err)
	}

	step := runtimeRefreshStepByName(report.Steps, "virtual_models")
	if step == nil {
		t.Fatalf("virtual_models step missing: %+v", report.Steps)
		return
	}
	if step.Status != control.RuntimeRefreshStatusSkipped {
		t.Fatalf("virtual_models step status = %q, want skipped; step=%+v", step.Status, *step)
	}
}

func TestRefreshRuntime_ReturnsGatewayErrorWhenContextCanceledBeforeAcquire(t *testing.T) {
	app := &App{}
	ch := app.runtimeRefreshSemaphore()
	ch <- struct{}{}
	defer func() { <-ch }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := app.RefreshRuntime(ctx)
	if err == nil {
		t.Fatal("RefreshRuntime() error = nil, want cancellation error")
	}

	var gatewayErr *core.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("RefreshRuntime() error = %T, want *core.GatewayError", err)
	}
	if gatewayErr.HTTPStatusCode() != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408", gatewayErr.HTTPStatusCode())
	}
	if gatewayErr.Provider != "runtime_refresh" {
		t.Fatalf("provider = %q, want runtime_refresh", gatewayErr.Provider)
	}
}

func TestRunRuntimeRefreshStepReturnsContextErrorWithoutAppendingStep(t *testing.T) {
	app := &App{}
	report := control.RuntimeRefreshReport{}

	err := app.runRuntimeRefreshStep(&report, "providers", func() runtimeRefreshStepResult {
		return runtimeRefreshStepResult{err: context.Canceled}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runRuntimeRefreshStep() error = %v, want context canceled", err)
	}
	if len(report.Steps) != 0 {
		t.Fatalf("steps = %+v, want none appended for context cancellation", report.Steps)
	}
}

func TestProviderRefreshIssueCountIncludesAvailabilityErrors(t *testing.T) {
	got := providerRefreshIssueCount([]providers.ProviderRuntimeSnapshot{
		{Name: "healthy"},
		{Name: "model-fetch", LastModelFetchError: " failed to fetch models "},
		{Name: "availability", LastAvailabilityError: " provider unavailable "},
		{Name: "both", LastModelFetchError: "fetch failed", LastAvailabilityError: "unavailable"},
	})
	if got != 3 {
		t.Fatalf("providerRefreshIssueCount() = %d, want 3", got)
	}
}

func runtimeRefreshStepByName(steps []control.RuntimeRefreshStep, name string) *control.RuntimeRefreshStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}
	return nil
}

func TestDashboardRuntimeConfig_ExposesFeatureAvailabilityFlags(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LogConfig{
			Enabled: true,
		},
		Usage: config.UsageConfig{
			Enabled: true,
		},

		Control: config.ControlConfig{
			Enabled: true,
			Listen:  "127.0.0.1:52181",
		},
		Cache: config.CacheConfig{
			Response: config.ResponseCacheConfig{
				Simple: &config.SimpleCacheConfig{},
			},
		},
	}

	values := controlRuntimeConfig(cfg, true)
	if got := values.LoggingEnabled; got != "on" {
		t.Fatalf("controlRuntimeConfig()[%q] = %q, want on", control.RuntimeConfigLoggingEnabled, got)
	}
	if got := values.UsageEnabled; got != "on" {
		t.Fatalf("controlRuntimeConfig()[%q] = %q, want on", control.RuntimeConfigUsageEnabled, got)
	}

	if got := values.CacheEnabled; got != "on" {
		t.Fatalf("controlRuntimeConfig()[%q] = %q, want on", control.RuntimeConfigCacheEnabled, got)
	}
}

func TestDashboardRuntimeConfig_HidesCacheAnalyticsWhenUsageDisabled(t *testing.T) {
	cfg := &config.Config{
		Usage: config.UsageConfig{
			Enabled: false,
		},
		Cache: config.CacheConfig{
			Response: config.ResponseCacheConfig{
				Simple: &config.SimpleCacheConfig{},
			},
		},
	}

	values := controlRuntimeConfig(cfg, false)
	if got := values.UsageEnabled; got != "off" {
		t.Fatalf("controlRuntimeConfig()[%q] = %q, want off", control.RuntimeConfigUsageEnabled, got)
	}
	if got := values.CacheEnabled; got != "off" {
		t.Fatalf("controlRuntimeConfig()[%q] = %q, want off", control.RuntimeConfigCacheEnabled, got)
	}
}

func TestApplyExtensionsSnapshotsRegistryIntoServerConfig(t *testing.T) {
	reg := &ext.Registry{}
	reg.RegisterRewriter(&staticRewriter{name: "r1"})
	reg.UseMiddleware(func(next echo.HandlerFunc) echo.HandlerFunc { return next })
	reg.RegisterRoutes(func(_ *echo.Echo) {})
	reg.AddPublicPaths("/sso/callback", "/sso/*")

	serverCfg := &server.Config{}
	applyExtensions(serverCfg, reg)

	if len(serverCfg.RequestRewriters) != 1 || serverCfg.RequestRewriters[0].Name() != "r1" {
		t.Errorf("RequestRewriters not copied: %+v", serverCfg.RequestRewriters)
	}
	if len(serverCfg.ExtraMiddleware) != 1 {
		t.Errorf("ExtraMiddleware not copied: %d entries", len(serverCfg.ExtraMiddleware))
	}
	if len(serverCfg.ExtraRoutes) != 1 {
		t.Errorf("ExtraRoutes not copied: %d entries", len(serverCfg.ExtraRoutes))
	}
	if len(serverCfg.ExtraAuthSkipPaths) != 2 {
		t.Errorf("ExtraAuthSkipPaths not copied: %v", serverCfg.ExtraAuthSkipPaths)
	}

	// A nil registry must leave the config untouched.
	empty := &server.Config{}
	applyExtensions(empty, nil)
	if empty.RequestRewriters != nil || empty.ExtraMiddleware != nil || empty.ExtraRoutes != nil || empty.ExtraAuthSkipPaths != nil {
		t.Error("nil registry must not modify server config")
	}
}

type staticRewriter struct{ name string }

func (r *staticRewriter) Name() string { return r.name }

func (r *staticRewriter) Rewrite(context.Context, ext.Input) (*ext.Result, error) {
	return nil, nil
}
