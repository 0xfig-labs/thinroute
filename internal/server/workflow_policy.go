package server

import (
	"context"

	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/gateway"
)

// RequestWorkflowPolicyResolver matches persisted workflow versions for requests.
type RequestWorkflowPolicyResolver = gateway.WorkflowPolicyResolver

func applyWorkflowPolicy(ctx context.Context, workflow *core.Workflow, resolver RequestWorkflowPolicyResolver, selector core.WorkflowSelector) error {
	return gateway.ApplyWorkflowPolicy(ctx, workflow, resolver, selector)
}
