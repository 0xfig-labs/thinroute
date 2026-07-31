// Package compatible provides a generic OpenAI-compatible provider that does
// not inherit OpenAI-specific request rewriting or header forwarding.
package compatible

import (
	"net/http"

	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/providers"
	"github.com/icehugh/thinroute/internal/providers/openai"
)

// Registration provides factory registration for the compatible provider type.
var Registration = providers.Registration{
	Type: "compatible",
	New:  New,
	Discovery: providers.DiscoveryConfig{
		RequireBaseURL: true,
	},
}

// Provider implements the core.Provider interface for any OpenAI-compatible
// upstream. Unlike the openai provider, it does not perform reasoning-parameter
// adaptation (max_completion_tokens / temperature) or forward OpenAI-specific
// request-ID headers.
type Provider struct {
	*openai.ChatCompatible
}

// passThroughBody serializes the chat request as-is, without OpenAI reasoning-
// parameter adaptation.
func passThroughBody(req *core.ChatRequest) (any, error) {
	return req, nil
}

// New creates a new compatible provider.
func New(cfg providers.ProviderConfig, opts providers.ProviderOptions) core.Provider {
	return &Provider{
		ChatCompatible: openai.NewChatCompatible(cfg.APIKey, opts, openai.CompatibleProviderConfig{
			ProviderName:    cfg.Name,
			BaseURL:         cfg.BaseURL,
			SetHeaders:      setHeaders,
			ChatRequestBody: passThroughBody,
		}),
	}
}

func setHeaders(req *http.Request, apiKey string) {
	providers.SetAuthHeaders(req, apiKey, providers.AuthHeaderConfig{
		AuthScheme: "Bearer ",
	})
}
