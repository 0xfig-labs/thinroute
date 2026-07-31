package control

import (
	"net/http"

	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/virtualmodels"
)

func featureUnavailableError(message string) error {
	return core.NewInvalidRequestErrorWithStatus(http.StatusServiceUnavailable, message, nil).
		WithCode("feature_unavailable")
}

// virtualModelWriteError maps store/provider failures to 502 and input failures to 400.
func virtualModelWriteError(err error) error {
	if err == nil {
		return nil
	}
	if virtualmodels.IsValidationError(err) {
		return core.NewInvalidRequestError(err.Error(), err)
	}
	return core.NewProviderError("virtual_models", http.StatusBadGateway, err.Error(), err)
}
