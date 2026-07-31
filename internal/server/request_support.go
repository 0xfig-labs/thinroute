package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
)

func requestIDFromContextOrHeader(req *http.Request) string {
	if req == nil {
		return ""
	}
	requestID := strings.TrimSpace(core.GetRequestID(req.Context()))
	if requestID != "" {
		return requestID
	}
	return strings.TrimSpace(req.Header.Get("X-Request-ID"))
}

func requestContextWithRequestID(req *http.Request) (context.Context, string) {
	if req == nil {
		requestID := uuid.NewString()
		return core.WithRequestID(context.Background(), requestID), requestID
	}

	requestID := requestIDFromContextOrHeader(req)
	if requestID == "" {
		requestID = uuid.NewString()
	}

	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("X-Request-ID", requestID)

	ctx := req.Context()
	if strings.TrimSpace(core.GetRequestID(ctx)) != requestID {
		ctx = core.WithRequestID(ctx, requestID)
		*req = *req.WithContext(ctx)
	}

	return ctx, requestID
}

// ponytail: stubs retained for compatibility after AuditControlCut cleanup.

func qualifyExecutedModel(workflow *core.Workflow, model, providerName string) string {
	if model == "" {
		return ""
	}
	if providerName != "" {
		return providerName + "/" + model
	}
	if workflow != nil {
		if pt := strings.TrimSpace(workflow.ProviderType); pt != "" {
			return pt + "/" + model
		}
	}
	return model
}

func markRequestFailoverUsed(c *echo.Context) {}

func providerNameFromWorkflow(workflow *core.Workflow) string {
	if workflow != nil && workflow.Resolution != nil {
		return strings.TrimSpace(workflow.Resolution.ProviderName)
	}
	return ""
}

func resolvedModelFromWorkflow(workflow *core.Workflow, fallback string) string {
	if workflow != nil {
		if m := strings.TrimSpace(workflow.RequestedQualifiedModel()); m != "" {
			return m
		}
	}
	return fallback
}

// ponytail: stub retained after AuditControlCut — ConfigLoader removed the original.
func marshalRequestBody(req any) ([]byte, error) {
	return json.Marshal(req)
}

// ponytail: stub retained after AuditControlCut — ConfigLoader removed the original.
func isClientDisconnectDuringDispatch(ctx context.Context, _ error) bool {
	return ctx != nil && ctx.Err() == context.Canceled
}
