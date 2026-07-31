package auditlog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/icehugh/thinroute/config"
)
// Result holds the initialized audit logger and its dependencies.
// The caller is responsible for calling Close() to release resources.
type Result struct {
	Logger LoggerInterface
}

// Close releases all resources held by the audit logger.
// Safe to call multiple times.
func (r *Result) Close() error {
	var errs []error
	if r.Logger != nil {
		if err := r.Logger.Close(); err != nil {
			errs = append(errs, fmt.Errorf("logger close: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %w", errors.Join(errs...))
	}
	return nil
}

// New creates an audit logger from configuration.
// Returns a Result containing the logger for lifecycle management.
// If logging is disabled in the config, returns a NoopLogger.
func New(ctx context.Context, cfg *config.Config) (*Result, error) {
	if !cfg.Logging.Enabled {
		return &Result{Logger: &NoopLogger{}}, nil
	}
	logCfg := buildLoggerConfig(cfg.Logging)
	return &Result{
		Logger: NewLogger(logCfg),
	}, nil
}


// buildLoggerConfig creates an auditlog.Config from config.LogConfig.
func buildLoggerConfig(logCfg config.LogConfig) Config {
	cfg := Config{
		Enabled:               logCfg.Enabled,
		BufferSize:            logCfg.BufferSize,
		FlushInterval:         time.Duration(logCfg.FlushInterval) * time.Second,
		RetentionDays:         logCfg.RetentionDays,
		OnlyModelInteractions: logCfg.OnlyModelInteractions,
	}

	// Apply defaults
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}

	return cfg
}
