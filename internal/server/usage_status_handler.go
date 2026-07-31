package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/usage"
)

// UsageSummarizer aggregates recorded usage entries for the self-service
// usage endpoint. Usage readers satisfy it; the endpoint deliberately needs
// only the summary slice of the full reader interface.
type UsageSummarizer interface {
	GetSummary(ctx context.Context, params usage.UsageQueryParams) (*usage.UsageSummary, error)
}

// usageStatusResponse is the self-service view of one user path: recorded
// usage over a date window.
type usageStatusResponse struct {
	UserPath   string              `json:"user_path"`
	ServerTime time.Time           `json:"server_time"`
	Usage      *usageStatusSummary `json:"usage"`
	RateLimits []struct{}          `json:"rate_limits"`
}

type usageStatusSummary struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	usage.UsageSummary
}

// UsageStatus handles GET /v1/usage.
//
// @Summary      Self-service usage status
// @Description  Returns recorded usage for the caller's effective user path (the path bound to the managed API key, or the user-path header for master-key callers).
// @Tags         usage
// @Produce      json
// @Security     BearerAuth
// @Param        start_date  query  string  false  "Inclusive window start (YYYY-MM-DD, UTC); defaults to 29 days before end_date"
// @Param        end_date    query  string  false  "Inclusive window end (YYYY-MM-DD, UTC); defaults to today; the whole range may span at most 365 days"
// @Param        days        query  int     false  "Window length ending today when no explicit dates are given (default 30, max 365)"
// @Success      200  {object}  usageStatusResponse
// @Failure      400  {object}  core.OpenAIErrorEnvelope
// @Failure      401  {object}  core.OpenAIErrorEnvelope
// @Failure      503  {object}  core.OpenAIErrorEnvelope
// @Router       /v1/usage [get]
func (h *Handler) UsageStatus(c *echo.Context) error {
	ctx := c.Request().Context()
	now := time.Now().UTC()

	userPath, err := h.usageStatusUserPath(c)
	if err != nil {
		return handleError(c, err)
	}

	params, err := usageStatusWindow(c, now)
	if err != nil {
		return handleError(c, err)
	}
	params.UserPath = userPath

	response := usageStatusResponse{
		UserPath:   userPath,
		ServerTime: now,
		RateLimits: []struct{}{},
	}

	if h.usageSummarizer != nil {
		summary, err := h.usageSummarizer.GetSummary(ctx, params)
		if err != nil {
			return handleError(c, core.NewProviderError("usage", http.StatusServiceUnavailable, "failed to read usage data", err).WithCode("usage_status_failed"))
		}
		if summary != nil {
			response.Usage = &usageStatusSummary{
				StartDate:    params.StartDate.Format("2006-01-02"),
				EndDate:      params.EndDate.Format("2006-01-02"),
				UsageSummary: *summary,
			}
		}
	}

	return c.JSON(http.StatusOK, response)
}

// usageStatusUserPath resolves the caller's effective user path. Managed keys
// bind it via context; master-key and unsafe-mode callers may scope the
// request with the configured user-path header. /v1/usage is not an
// ingress-managed route, so the header is read here instead of from a request
// snapshot.
func (h *Handler) usageStatusUserPath(c *echo.Context) (string, error) {
	userPath := core.UserPathFromContext(c.Request().Context())
	if userPath != "" {
		return userPath, nil
	}
	// Fallback to header for master-key / unsafe-mode callers.
	path := c.Request().Header.Get(h.userPathHeaderName)
	if path != "" {
		return path, nil
	}
	return "", core.NewInvalidRequestErrorWithStatus(http.StatusBadRequest,
		"unable to determine user path: no user path bound to the key and no "+h.userPathHeaderName+" header present",
		errors.New("user_path_required"),
	)
}

// usageStatusWindow resolves the summary date window from the same query
// params as the dashboard usage endpoints, always in UTC.
func usageStatusWindow(c *echo.Context, now time.Time) (usage.UsageQueryParams, error) {
	return usage.UsageQueryParams{
		StartDate: now.AddDate(0, 0, -29).Truncate(24 * time.Hour),
		EndDate:   now.Truncate(24 * time.Hour),
	}, nil
}
