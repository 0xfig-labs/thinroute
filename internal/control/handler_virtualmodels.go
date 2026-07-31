package control

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/icehugh/thinroute/internal/virtualmodels"
)

// ListVirtualModels returns the immutable virtual-model configuration together
// with its current runtime state.
func (h *Handler) ListVirtualModels(c *echo.Context) error {
	if h.virtualModels == nil {
		return handleError(c, featureUnavailableError("virtual models feature is unavailable"))
	}
	views := h.virtualModels.ListViews()
	if views == nil {
		views = []virtualmodels.View{}
	}
	return c.JSON(http.StatusOK, views)
}
