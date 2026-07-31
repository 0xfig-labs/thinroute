package control

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/usage"
)

// maxUsageLogLimit caps the page size accepted by the usage log endpoint and
// matches the value documented in the @Param limit annotation below.
const maxUsageLogLimit = 200

// defaultUsageLogLimit is the effective page size when the caller omits limit.
// It mirrors the reader's pagination default so the disabled-reader fast path
// reports the same limit an enabled reader would.
const defaultUsageLogLimit = 50

// UsageSummary handles GET /control/v1/usage/summary
//
// @Summary      Get usage summary
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Success      200  {object}  usage.UsageSummary
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/summary [get]
func (h *Handler) UsageSummary(c *echo.Context) error {
	// Validate request shape before the disabled-reader fast path so callers
	// always get a 400 for malformed inputs, regardless of wiring.
	params, err := parseUsageParams(c)
	if err != nil {
		return handleError(c, err)
	}

	if h.usageReader == nil {
		return c.JSON(http.StatusOK, usage.UsageSummary{})
	}

	summary, err := h.usageReader.GetSummary(c.Request().Context(), params)
	if err != nil {
		return handleError(c, err)
	}
	if summary == nil {
		summary = &usage.UsageSummary{}
	}

	return c.JSON(http.StatusOK, summary)
}

func usageSliceResponse[T any](
	c *echo.Context,
	reader usage.UsageReader,
	fetch func(context.Context, usage.UsageQueryParams) ([]T, error),
) error {
	// Validate before the disabled-reader fast path so malformed query
	// params produce a 400 even when usage tracking is disabled.
	params, err := parseUsageParams(c)
	if err != nil {
		return handleError(c, err)
	}

	if reader == nil {
		return c.JSON(http.StatusOK, []T{})
	}

	values, err := fetch(c.Request().Context(), params)
	if err != nil {
		return handleError(c, err)
	}
	if values == nil {
		values = []T{}
	}
	return c.JSON(http.StatusOK, values)
}

// DailyUsage handles GET /control/v1/usage/daily
//
// @Summary      Get usage breakdown by period
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        interval    query     string  false  "Grouping interval: daily, weekly, monthly, yearly (default daily)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Success      200  {array}   usage.DailyUsage
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/daily [get]
func (h *Handler) DailyUsage(c *echo.Context) error {
	return usageSliceResponse(c, h.usageReader, func(ctx context.Context, params usage.UsageQueryParams) ([]usage.DailyUsage, error) {
		return h.usageReader.GetDailyUsage(ctx, params)
	})
}

// UsageByModel handles GET /control/v1/usage/models
//
// @Summary      Get usage breakdown by model
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Success      200  {array}   usage.ModelUsage
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/models [get]
func (h *Handler) UsageByModel(c *echo.Context) error {
	return usageSliceResponse(c, h.usageReader, func(ctx context.Context, params usage.UsageQueryParams) ([]usage.ModelUsage, error) {
		return h.usageReader.GetUsageByModel(ctx, params)
	})
}

// UsageByUserPath handles GET /control/v1/usage/user-paths
//
// @Summary      Get usage breakdown by user path
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Success      200  {array}   usage.UserPathUsage
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/user-paths [get]
func (h *Handler) UsageByUserPath(c *echo.Context) error {
	return usageSliceResponse(c, h.usageReader, func(ctx context.Context, params usage.UsageQueryParams) ([]usage.UserPathUsage, error) {
		return h.usageReader.GetUsageByUserPath(ctx, params)
	})
}

// UsageByLabel handles GET /control/v1/usage/labels
//
// @Summary      Get usage breakdown by request label
// @Description  Returns per-label token and cost aggregates. Requests carrying
// @Description  several labels count once per label, so rows overlap and do
// @Description  not sum to the period totals. Unlabelled requests are omitted.
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Success      200  {array}   usage.LabelUsage
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/labels [get]
func (h *Handler) UsageByLabel(c *echo.Context) error {
	return usageSliceResponse(c, h.usageReader, func(ctx context.Context, params usage.UsageQueryParams) ([]usage.LabelUsage, error) {
		return h.usageReader.GetUsageByLabel(ctx, params)
	})
}

// UsageLog handles GET /control/v1/usage/log
//
// @Summary      Get paginated usage log entries
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (default uncached)"
// @Param        search      query     string  false  "Search across model, provider, request_id, provider_id"
// @Param        limit       query     int     false  "Page size (default 50, max 200)"
// @Param        offset      query     int     false  "Offset for pagination"
// @Success      200  {object}  usage.UsageLogResult
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/log [get]
func (h *Handler) UsageLog(c *echo.Context) error {
	// Validate request shape before the disabled-reader fast path so callers
	// always get a 400 for malformed inputs, regardless of wiring.
	baseParams, err := parseUsageParams(c)
	if err != nil {
		return handleError(c, err)
	}

	params := usage.UsageLogParams{
		UsageQueryParams: baseParams,
		Search:           c.QueryParam("search"),
	}

	if l := c.QueryParam("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			return handleError(c, core.NewInvalidRequestError("invalid limit, expected positive integer", nil))
		}
		if parsed > maxUsageLogLimit {
			return handleError(c, core.NewInvalidRequestError("invalid limit parameter: limit must be between 1 and 200", nil))
		}
		params.Limit = parsed
	}
	if o := c.QueryParam("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err != nil || parsed < 0 {
			return handleError(c, core.NewInvalidRequestError("invalid offset, expected non-negative integer", nil))
		}
		params.Offset = parsed
	}

	if h.usageReader == nil {
		// Echo the effective pagination so the response matches the enabled-reader
		// contract. Returning limit:0 here would make the client send limit=0 on
		// its next request, which fails validation above with a 400.
		limit := params.Limit
		if limit <= 0 {
			limit = defaultUsageLogLimit
		}
		return c.JSON(http.StatusOK, usage.UsageLogResult{
			Entries: []usage.UsageLogEntry{},
			Limit:   limit,
			Offset:  params.Offset,
		})
	}

	result, err := h.usageReader.GetUsageLog(c.Request().Context(), params)
	if err != nil {
		return handleError(c, err)
	}
	if result == nil {
		result = &usage.UsageLogResult{Entries: []usage.UsageLogEntry{}}
	}
	if result.Entries == nil {
		result.Entries = []usage.UsageLogEntry{}
	}
	for i := range result.Entries {
		usage.EnrichUsageLogEntry(&result.Entries[i])
	}

	return c.JSON(http.StatusOK, result)
}

// CacheOverview handles GET /control/v1/cache/overview
//
// @Summary      Get cached-only usage overview
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        days        query     int     false  "Number of days (default 30)"
// @Param        start_date  query     string  false  "Start date (YYYY-MM-DD)"
// @Param        end_date    query     string  false  "End date (YYYY-MM-DD)"
// @Param        interval    query     string  false  "Grouping interval: daily, weekly, monthly, yearly (default daily)"
// @Param        model       query     string  false  "Filter by exact model name"
// @Param        provider    query     string  false  "Filter by provider name or provider type"
// @Param        label       query     string  false  "Filter by request label (exact match)"
// @Param        user_path   query     string  false  "Filter by tracked user path subtree"
// @Param        cache_mode  query     string  false  "Cache mode filter: uncached, cached, all (cache overview always uses cached mode)"
// @Success      200  {object}  usage.CacheOverview
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Failure      503  {object}  core.GatewayError
// @Router       /control/v1/cache/overview [get]
func (h *Handler) CacheOverview(c *echo.Context) error {
	// Feature-gate check stays first: this endpoint is unavailable when cache
	// analytics is disabled.
	if strings.TrimSpace(h.runtimeConfig.CacheEnabled) != "on" {
		return handleError(c, featureUnavailableError("cache analytics is unavailable"))
	}

	// Validate request shape before the disabled-reader fast path so callers
	// always get a 400 for malformed inputs, regardless of wiring.
	params, err := parseUsageParams(c)
	if err != nil {
		return handleError(c, err)
	}
	params.CacheMode = usage.CacheModeCached

	if h.usageReader == nil {
		return c.JSON(http.StatusOK, usage.CacheOverview{
			Daily: []usage.CacheOverviewDaily{},
		})
	}

	overview, err := h.usageReader.GetCacheOverview(c.Request().Context(), params)
	if err != nil {
		return handleError(c, err)
	}
	if overview == nil {
		overview = &usage.CacheOverview{}
	}
	if overview.Daily == nil {
		overview.Daily = []usage.CacheOverviewDaily{}
	}

	return c.JSON(http.StatusOK, overview)
}

// TokenThroughput handles GET /control/v1/usage/throughput.
//
// @Summary      Get the live token-throughput window
// @Description  Returns a fixed, trailing window of token-volume buckets
// @Description  (input / output / prompt-cached / locally-cached) at the
// @Description  requested granularity, for the overview live chart.
// @Tags         control
// @Produce      json
// @Security     BearerAuth
// @Param        granularity  query     string  true   "Bucket granularity: second, minute, hour, day"
// @Success      200  {object}  usage.TokenThroughput
// @Failure      400  {object}  core.GatewayError
// @Failure      401  {object}  core.GatewayError
// @Router       /control/v1/usage/throughput [get]
func (h *Handler) TokenThroughput(c *echo.Context) error {
	gran, err := usage.ParseThroughputGranularity(c.QueryParam("granularity"))
	if err != nil {
		return handleError(c, core.NewInvalidRequestError(err.Error(), nil))
	}

	now := time.Now().UTC()
	// Align buckets to the dashboard's timezone so day buckets start at local
	// midnight (matching the Daily chart), not UTC.
	_, location := usageTimeZone(c)
	if location == nil {
		location = time.UTC
	}
	_, offsetSeconds := now.In(location).Zone()
	offset := int64(offsetSeconds)

	if h.usageReader == nil {
		return c.JSON(http.StatusOK, usage.EmptyTokenThroughput(gran, now, offset))
	}

	result, err := h.usageReader.GetTokenThroughput(c.Request().Context(), gran, now, offset)
	if err != nil {
		return handleError(c, err)
	}
	if result == nil {
		result = usage.EmptyTokenThroughput(gran, now, offset)
	}
	return c.JSON(http.StatusOK, result)
}
