//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/0xfig-labs/thinroute/internal/providers"
	"github.com/0xfig-labs/thinroute/internal/server"
	"github.com/0xfig-labs/thinroute/internal/usage"
)

type e2eServerOptions struct {
	usageLogger     usage.LoggerInterface
	budgetChecker   server.BudgetChecker
	pricingResolver usage.PricingResolver
	providerType    string
	// registry, when set, replaces the fixture's own registry so tests can
	// share it with collaborators built around the same catalog (virtual
	// models, rate-limit capacity probes).
	registry *providers.ModelRegistry
	// modelResolver and failoverResolver mirror the app wiring for alias
	// resolution and translated-route failover.
	modelResolver    server.RequestModelResolver
	failoverResolver server.RequestFailoverResolver
}

type e2eUsageFixture struct {
	reader    usage.UsageReader
	logger    *usage.Logger
	closeOnce sync.Once
	closeErr  error
}

// setupAuthServer creates a new server instance.
func setupAuthServer(t *testing.T) *server.Server {
	t.Helper()
	return setupE2EServer(t, e2eServerOptions{})
}

func setupE2EServer(t *testing.T, opts e2eServerOptions) *server.Server {
	t.Helper()

	registry := opts.registry
	if registry == nil {
		registry = setupE2ERegistry(t, opts.providerType)
	}
	router, err := providers.NewRouter(registry)
	require.NoError(t, err, "failed to create router")

	cfg := &server.Config{
		UsageLogger:      opts.usageLogger,
		BudgetChecker:    opts.budgetChecker,
		PricingResolver:  opts.pricingResolver,
		ModelResolver:    opts.modelResolver,
		FailoverResolver: opts.failoverResolver,
	}

	return server.New(router, cfg)
}

func setupE2ERegistry(t *testing.T, providerType string) *providers.ModelRegistry {
	t.Helper()

	if providerType == "" {
		providerType = "test"
	}

	testProvider := NewTestProvider(mockLLMURL, "sk-test-key-12345")
	registry := providers.NewModelRegistry()
	registry.RegisterProviderWithType(testProvider, providerType)

	require.NoError(t, registry.Initialize(context.Background()), "failed to initialize registry")
	return registry
}

func setupSQLiteUsageFixture(t *testing.T) *e2eUsageFixture {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	store, err := usage.NewSQLiteStore(db, 0)
	require.NoError(t, err)

	reader, err := usage.NewSQLiteReader(db)
	require.NoError(t, err)

	cfg := usage.DefaultConfig()
	cfg.Enabled = true
	cfg.BufferSize = 10
	cfg.FlushInterval = time.Hour
	logger := usage.NewLogger(store, cfg)

	fixture := &e2eUsageFixture{
		reader: reader,
		logger: logger,
	}
	t.Cleanup(func() {
		fixture.flush(t)
	})

	return fixture
}

func (f *e2eUsageFixture) flush(t *testing.T) {
	t.Helper()

	f.closeOnce.Do(func() {
		f.closeErr = f.logger.Close()
	})
	require.NoError(t, f.closeErr)
}
