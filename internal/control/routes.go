package control

import "github.com/labstack/echo/v5"

// RouteRegistrar is the subset of *echo.Group / *echo.Echo that RegisterRoutes
// uses. Decoupling from a concrete echo type keeps the         control package useful for
// callers that want to mount the API under a different path prefix or wrap the
// routes with extra middleware.
type RouteRegistrar interface {
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) echo.RouteInfo
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) echo.RouteInfo
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) echo.RouteInfo
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) echo.RouteInfo
}

// RegisterRoutes mounts the control REST API on the given route group.
// Callers typically pass an *echo.Group rooted at /control.
func (h *Handler) RegisterRoutes(g RouteRegistrar) {
	g.GET("/runtime/config", h.RuntimeConfig)
	g.POST("/runtime/refresh", h.RefreshRuntime)
	g.GET("/cache/overview", h.CacheOverview)

	g.GET("/usage/summary", h.UsageSummary)
	g.GET("/usage/daily", h.DailyUsage)
	g.GET("/usage/models", h.UsageByModel)

	g.GET("/providers", h.ProviderStatus)
	g.GET("/providers/cooldown", h.ProviderCooldowns)
	g.POST("/providers/:name/test", h.TestProvider)
	g.POST("/providers/:name/refresh", h.SyncProviderModels)

	g.GET("/models", h.ListModels)
	g.GET("/models/categories", h.ListCategories)

	g.GET("/virtual-models", h.ListVirtualModels)
}
