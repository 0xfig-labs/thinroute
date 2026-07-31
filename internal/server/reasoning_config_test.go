package server

import (
	"testing"

	"github.com/goccy/go-json"

	"github.com/0xfig-labs/thinroute/internal/core"
)

func TestDisableReasoningStripsTypedAndProviderFields(t *testing.T) {
	extra := core.UnknownJSONFieldsFromMap(map[string]json.RawMessage{
		"reasoning_effort": json.RawMessage(`"high"`),
		"enable_thinking":  json.RawMessage(`true`),
		"temperature":      json.RawMessage(`0.5`),
	})
	chat := &core.ChatRequest{Reasoning: &core.Reasoning{Effort: "high"}, ExtraFields: extra}
	responses := &core.ResponsesRequest{Reasoning: &core.Reasoning{Effort: "high"}, ExtraFields: extra}

	disableChatReasoning(chat)
	disableResponsesReasoning(responses)

	for name, got := range map[string]struct {
		reasoning *core.Reasoning
		fields    core.UnknownJSONFields
	}{
		"chat":      {chat.Reasoning, chat.ExtraFields},
		"responses": {responses.Reasoning, responses.ExtraFields},
	} {
		if got.reasoning != nil {
			t.Fatalf("%s reasoning was not removed", name)
		}
		if got.fields.Lookup("reasoning_effort") != nil || got.fields.Lookup("enable_thinking") != nil {
			t.Fatalf("%s provider reasoning fields were not removed", name)
		}
		if got.fields.Lookup("temperature") == nil {
			t.Fatalf("%s unrelated field was removed", name)
		}
	}
}

func TestShouldDisableReasoningMatchesRequestedOrResolvedModel(t *testing.T) {
	service := &translatedInferenceService{
		disableReasoningModels: normalizeReasoningDisabledModels([]string{"fast", "deepseek/deepseek-v4-flash"}),
	}
	for _, workflow := range []*core.Workflow{
		{Resolution: &core.RequestModelResolution{Requested: core.NewRequestedModelSelector("fast", "")}},
		{Resolution: &core.RequestModelResolution{ResolvedSelector: core.ModelSelector{Provider: "deepseek", Model: "deepseek-v4-flash"}}},
	} {
		if !service.shouldDisableReasoning(workflow) {
			t.Fatalf("expected reasoning disabled for workflow %+v", workflow.Resolution)
		}
	}
}
