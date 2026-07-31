package auditlog

import (
	"context"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/0xfig-labs/thinroute/internal/core"
)

// This file is the enrichment API handlers use to attach data to the
// audit entry stored on the request context. The EnrichLogEntry* variants
// mutate an entry directly for executors that run outside Echo middleware state.

// entryFromContext returns the live audit entry stored on the request context,
// or nil when audit logging is inactive for the request.
func entryFromContext(c *echo.Context) *LogEntry {
	if c == nil {
		return nil
	}
	entry, ok := c.Get(string(LogEntryKey)).(*LogEntry)
	if !ok {
		return nil
	}
	return entry
}

// EnrichEntry retrieves the log entry from context for enrichment by handlers.
// This allows handlers to add model and provider information.
func EnrichEntry(c *echo.Context, model, provider string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	entry.RequestedModel = model
	entry.Provider = provider
}

// EnrichEntryWithRequestedModel attaches early requested-model metadata to the
// audit entry before the final workflow policy has been resolved.
func EnrichEntryWithRequestedModel(c *echo.Context, requestedModel string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return
	}
	entry.RequestedModel = requestedModel
}

// EnrichEntryWithWorkflow attaches workflow metadata to the audit entry.
func EnrichEntryWithWorkflow(c *echo.Context, workflow *core.Workflow) {
	syncRequestWorkflow(c, workflow)

	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	enrichEntryWithWorkflow(entry, workflow)
}

func syncRequestWorkflow(c *echo.Context, workflow *core.Workflow) {
	if c == nil || workflow == nil {
		return
	}
	req := c.Request()
	if req == nil {
		return
	}
	ctx := req.Context()
	if core.GetWorkflow(ctx) == workflow {
		return
	}
	c.SetRequest(req.WithContext(core.WithWorkflow(ctx, workflow)))
}

// EnrichLogEntryWithWorkflow attaches workflow metadata directly to
// an existing log entry.
func EnrichLogEntryWithWorkflow(entry *LogEntry, workflow *core.Workflow) {
	enrichEntryWithWorkflow(entry, workflow)
}

// EnrichEntryWithResolvedRoute attaches the final executed route to the
// audit entry after execution resolved to a concrete provider/model.
func EnrichEntryWithResolvedRoute(c *echo.Context, resolvedModel, providerType, providerName string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	enrichEntryWithResolvedRoute(entry, resolvedModel, providerType, providerName)
}

// EnrichLogEntryWithResolvedRoute attaches the final executed route directly to
// an existing audit log entry.
func EnrichLogEntryWithResolvedRoute(entry *LogEntry, resolvedModel, providerType, providerName string) {
	enrichEntryWithResolvedRoute(entry, resolvedModel, providerType, providerName)
}

// EnrichEntryWithFailover records the configured failover selector.
func EnrichEntryWithFailover(c *echo.Context, targetModel string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	enrichEntryWithFailover(entry, targetModel)
}

// EnrichLogEntryWithFailover attaches failover redirect metadata directly to an
// existing audit log entry.
func EnrichLogEntryWithFailover(entry *LogEntry, targetModel string) {
	enrichEntryWithFailover(entry, targetModel)
}

// EnrichEntryWithAttempts attaches provider attempt summaries to the
// audit entry.
func EnrichEntryWithAttempts(c *echo.Context, attempts []AttemptSnapshot) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	enrichEntryWithAttempts(entry, attempts)
}

// EnrichLogEntryWithAttempts attaches provider attempt summaries directly to an
// existing audit log entry.
func EnrichLogEntryWithAttempts(entry *LogEntry, attempts []AttemptSnapshot) {
	enrichEntryWithAttempts(entry, attempts)
}

// EnrichEntryWithCacheType attaches cache-hit metadata to the audit entry.
func EnrichEntryWithCacheType(c *echo.Context, cacheType string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(cacheType)) != CacheTypeExact {
		return
	}
	entry.CacheType = CacheTypeExact
}

// EnrichLogEntryWithRequestContext is retained for compatibility with the
// request pipeline; removed identity and label enrichment is intentionally empty.
func EnrichLogEntryWithRequestContext(_ *LogEntry, _ context.Context) {}

// EnrichEntryWithError adds error information to the log entry.
func EnrichEntryWithError(c *echo.Context, errorType, errorMessage string, errorCode ...string) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	entry.ErrorType = errorType
	data := ensureLogData(entry)
	data.ErrorMessage = errorMessage
	if len(errorCode) > 0 {
		if code := strings.TrimSpace(errorCode[0]); code != "" {
			data.ErrorCode = code
		}
	}
}

// EnrichEntryWithStream marks the log entry as a streaming request.
func EnrichEntryWithStream(c *echo.Context, stream bool) {
	entry := entryFromContext(c)
	if entry == nil {
		return
	}
	entry.Stream = stream
}

func enrichEntryWithWorkflow(entry *LogEntry, workflow *core.Workflow) {
	if entry == nil || workflow == nil {
		return
	}

	if requestID := strings.TrimSpace(workflow.RequestID); requestID != "" {
		entry.RequestID = requestID
	}
	if requestedModel := workflow.RequestedQualifiedModel(); requestedModel != "" {
		entry.RequestedModel = requestedModel
	}

	// When a runtime failover already recorded the actual executed route
	// (resolved_model/provider, via EnrichEntryWithResolvedRoute), the workflow
	// here only carries the planned primary resolution — so it must not clobber
	// the real route the request ended up taking.
	failoverRecorded := entry.Data != nil && entry.Data.Failover != nil
	executedResolvedModel := failoverRecorded && strings.TrimSpace(entry.ResolvedModel) != ""
	executedProvider := failoverRecorded && strings.TrimSpace(entry.Provider) != ""
	executedProviderName := failoverRecorded && strings.TrimSpace(entry.ProviderName) != ""

	if resolvedModel := resolvedModelForAuditLog(workflow); resolvedModel != "" && !executedResolvedModel {
		entry.ResolvedModel = resolvedModel
	}
	if workflow.Mode == core.ExecutionModePassthrough && workflow.Passthrough != nil {
		if model := strings.TrimSpace(workflow.Passthrough.Model); model != "" {
			entry.RequestedModel = model
		}
	}
	if !executedProvider {
		if providerType := strings.TrimSpace(workflow.ProviderType); providerType != "" {
			entry.Provider = providerType
		} else if workflow.Resolution != nil && strings.TrimSpace(workflow.Resolution.ProviderType) != "" {
			entry.Provider = strings.TrimSpace(workflow.Resolution.ProviderType)
		}
	}
	if workflow.Resolution != nil {
		if providerName := strings.TrimSpace(workflow.Resolution.ProviderName); providerName != "" && !executedProviderName {
			entry.ProviderName = providerName
		}
		entry.AliasUsed = workflow.Resolution.AliasApplied
	}
	if versionID := strings.TrimSpace(workflow.WorkflowVersionID()); versionID != "" {
		entry.WorkflowVersionID = versionID
	}
	if workflow.Policy != nil {
		ensureLogData(entry).WorkflowFeatures = &WorkflowFeaturesSnapshot{
			Cache:      workflow.Policy.Features.Cache,
			Audit:      workflow.Policy.Features.Audit,
			Usage:      workflow.Policy.Features.Usage,
			Budget:     workflow.Policy.Features.Budget,
			Guardrails: workflow.Policy.Features.Guardrails,
			Failover:   workflow.Policy.Features.Failover,
		}
	}
}

func resolvedModelForAuditLog(workflow *core.Workflow) string {
	if workflow == nil || workflow.Resolution == nil {
		return ""
	}
	model := strings.TrimSpace(workflow.Resolution.ResolvedSelector.Model)
	if model == "" {
		return ""
	}
	if providerName := strings.TrimSpace(workflow.Resolution.ProviderName); providerName != "" {
		return providerName + "/" + model
	}
	if provider := strings.TrimSpace(workflow.Resolution.ResolvedSelector.Provider); provider != "" {
		return provider + "/" + model
	}
	return model
}

func enrichEntryWithResolvedRoute(entry *LogEntry, resolvedModel, providerType, providerName string) {
	if entry == nil {
		return
	}
	if resolvedModel = strings.TrimSpace(resolvedModel); resolvedModel != "" {
		entry.ResolvedModel = resolvedModel
	}
	if providerType = strings.TrimSpace(providerType); providerType != "" {
		entry.Provider = providerType
	}
	if providerName = strings.TrimSpace(providerName); providerName != "" {
		entry.ProviderName = providerName
	}
}

func enrichEntryWithFailover(entry *LogEntry, targetModel string) {
	if entry == nil {
		return
	}
	targetModel = strings.TrimSpace(targetModel)
	if targetModel == "" {
		return
	}
	ensureLogData(entry).Failover = &FailoverSnapshot{
		TargetModel: targetModel,
	}
}

func enrichEntryWithAttempts(entry *LogEntry, attempts []AttemptSnapshot) {
	if entry == nil {
		return
	}
	normalized := normalizeAttemptSnapshots(attempts)
	if len(normalized) == 0 {
		return
	}
	ensureLogData(entry).Attempts = normalized
}

// EnrichEntryWithCachedStreamResponse is retained for the request pipeline;
// response body capture is intentionally disabled.
func EnrichEntryWithCachedStreamResponse(_ *echo.Context, _ string, _ []byte) {}

// IsAudioContentType reports whether audio logging is enabled.
func IsAudioContentType(_ string) bool { return false }

// GetStreamEntryFromContext returns nil: stream entry tracking removed.
func GetStreamEntryFromContext(_ *echo.Context) *LogEntry { return nil }

// CaptureAttemptResponseBody is a no-op: attempt body capture removed.
func CaptureAttemptResponseBody(_ interface{}, _ []byte) {}

// RedactAttemptResponseHeaders is a no-op: attempt header capture removed.
func RedactAttemptResponseHeaders(_ interface{}, _ map[string]string) {}

// ensureLogData returns entry.Data, initializing it when nil.
func ensureLogData(entry *LogEntry) *LogData {
	if entry.Data == nil {
		entry.Data = &LogData{}
	}
	return entry.Data
}
