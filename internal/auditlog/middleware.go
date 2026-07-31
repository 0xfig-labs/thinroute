package auditlog

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/icehugh/thinroute/internal/core"
)

// Middleware creates an Echo middleware for audit logging.
// It captures request metadata at the start and writes the log entry
// asynchronously after the handler completes.
func Middleware(logger LoggerInterface) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// Skip if logging is disabled
			if logger == nil || !logger.Config().Enabled {
				return next(c)
			}

			cfg := logger.Config()

			// Skip non-model paths if OnlyModelInteractions is enabled
			if cfg.OnlyModelInteractions && !core.IsModelInteractionPath(c.Request().URL.Path) {
				return next(c)
			}

			// Short-circuit when an upstream component has already
			// populated the context with an Audit=false workflow before next(c).
			if !auditEnabledForContext(c.Request().Context()) {
				return next(c)
			}

			start := time.Now()
			req := c.Request()

			requestID := req.Header.Get("X-Request-ID")

			// Create initial log entry
			entry := &LogEntry{
				ID:        uuid.NewString(),
				Timestamp: start,
				RequestID: requestID,
				ClientIP:  c.RealIP(),
				Method:    req.Method,
				Path:      req.URL.Path,
				Data: &LogData{
					UserAgent: req.UserAgent(),
				},
			}

			// Store entry in context for potential enrichment by handlers
			c.Set(string(LogEntryKey), entry)

			// Execute the handler
			err := next(c)

			applyWorkflow(entry, c.Request().Context())

			if !auditEnabledForContext(c.Request().Context()) {
				return err
			}

			// Calculate duration
			entry.DurationNs = time.Since(start).Nanoseconds()

			// ResolveResponseStatus applies Echo v5 precedence rules
			_, entry.StatusCode = echo.ResolveResponseStatus(c.Response(), err)

			// Write log entry asynchronously
			logger.Write(entry)

			return err
		}
	}
}

func applyWorkflow(entry *LogEntry, ctx context.Context) {
	if entry == nil || ctx == nil {
		return
	}
	if workflow := core.GetWorkflow(ctx); workflow != nil {
		enrichEntryWithWorkflow(entry, workflow)
	}
}

func auditEnabledForContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if workflow := core.GetWorkflow(ctx); workflow != nil {
		return workflow.AuditEnabled()
	}
	return true
}
