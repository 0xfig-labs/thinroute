package server

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
)

type BudgetChecker interface {
	Check(ctx context.Context, userPath string, now time.Time) error
}

func enforceBudget(c *echo.Context, checker BudgetChecker) error {
	if checker == nil || c == nil || c.Request() == nil {
		return nil
	}
	return enforceBudgetForContext(c.Request().Context(), checker)
}

func enforceBudgetForContext(ctx context.Context, checker BudgetChecker) error {
	if checker == nil || ctx == nil {
		return nil
	}
	if workflow := core.GetWorkflow(ctx); workflow != nil && !workflow.BudgetEnabled() {
		return nil
	}
	userPath := core.UserPathFromContext(ctx)
	if userPath == "" {
		userPath = "/"
	}
	if err := checker.Check(ctx, userPath, time.Now().UTC()); err != nil {
		return budgetCheckError(err)
	}
	return nil
}

func budgetCheckError(err error) error {
	return core.NewProviderError("budget", http.StatusServiceUnavailable, "budget check failed", err).
		WithCode("budget_check_failed")
}
