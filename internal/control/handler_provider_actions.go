package control

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
)

type testProviderResponse struct {
	OK     bool   `json:"ok"`
	Models int    `json:"models,omitempty"`
	Error  string `json:"error,omitempty"`
}

type syncModelsResponse struct {
	Name   string `json:"name"`
	Models int    `json:"models"`
}

// TestProvider verifies connectivity by refreshing the configured provider's
// model inventory. Provider definitions are immutable runtime configuration;
// the control plane never reads credentials from a second persistence layer.
func (h *Handler) TestProvider(c *echo.Context) error {
	if h.registry == nil {
		return handleError(c, featureUnavailableError("provider registry is unavailable"))
	}
	count, err := h.registry.RefreshProviderModels(c.Request().Context(), c.Param("name"))
	if err != nil {
		return c.JSON(http.StatusOK, testProviderResponse{OK: false, Error: err.Error()})
	}
	return c.JSON(http.StatusOK, testProviderResponse{OK: true, Models: count})
}

// SyncProviderModels refreshes one configured provider's discovered models.
func (h *Handler) SyncProviderModels(c *echo.Context) error {
	if h.registry == nil {
		return handleError(c, core.NewProviderError("model_registry", http.StatusServiceUnavailable, "provider registry is unavailable", nil))
	}
	name := c.Param("name")
	count, err := h.registry.RefreshProviderModels(c.Request().Context(), name)
	if err != nil {
		return handleError(c, err)
	}
	return c.JSON(http.StatusOK, syncModelsResponse{Name: name, Models: count})
}
