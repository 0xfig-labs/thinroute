package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
)

type imageGenerationRequest struct {
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"`
}

// ImageGenerations forwards the OpenAI-compatible image generation request to
// the resolved provider. The body remains provider-native so vendor-specific
// fields such as ratio and extra_body are preserved.
func (h *Handler) ImageGenerations(c *echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return handleError(c, core.NewInvalidRequestError("invalid request body: "+err.Error(), err))
	}
	var selector imageGenerationRequest
	if err := json.Unmarshal(body, &selector); err != nil || strings.TrimSpace(selector.Model) == "" {
		return handleError(c, core.NewInvalidRequestError("model is required", err))
	}
	workflow, err := ensureTranslatedRequestWorkflowWithAuthorizer(
		c, h.provider, h.modelResolver, h.modelAuthorizer, h.workflowPolicyResolver,
		&selector.Model, &selector.Provider,
	)
	if err != nil {
		return handleError(c, err)
	}
	providerType := strings.TrimSpace(workflow.ProviderType)
	passthroughProvider, ok := h.provider.(core.RoutablePassthrough)
	if !ok {
		return handleError(c, core.NewInvalidRequestError("image generation is not supported by the current provider router", nil))
	}
	ctx, _ := requestContextWithRequestID(c.Request())
	resp, err := passthroughProvider.Passthrough(ctx, providerType, &core.PassthroughRequest{
		Method:       http.MethodPost,
		Endpoint:     "/images/generations",
		Body:         io.NopCloser(bytes.NewReader(body)),
		Headers:      buildPassthroughHeaders(ctx, c.Request().Header),
		ProviderName: providerNameFromWorkflow(workflow),
	})
	if err != nil {
		return handleError(c, err)
	}
	info := &core.PassthroughRouteInfo{
		Provider:    providerType,
		RawEndpoint: "images/generations",
		AuditPath:   c.Request().URL.Path,
		Model:       selector.Model,
	}
	service := passthroughService{
		provider:        h.provider,
		modelAuthorizer: h.modelAuthorizer,
		logger:          h.logger,
		usageLogger:     h.usageLogger,
		budgetChecker:   h.budgetChecker,
		pricingResolver: h.pricingResolver,
	}
	return service.proxyPassthroughResponse(c, providerType, providerNameFromWorkflow(workflow), "/images/generations", info, resp)
}
