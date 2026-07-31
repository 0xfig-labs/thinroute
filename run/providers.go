package run

import (
	"github.com/0xfig-labs/thinroute/config"
	"github.com/0xfig-labs/thinroute/internal/observability"
	"github.com/0xfig-labs/thinroute/internal/providers"
	"github.com/0xfig-labs/thinroute/internal/providers/anthropic"
	"github.com/0xfig-labs/thinroute/internal/providers/azure"
	"github.com/0xfig-labs/thinroute/internal/providers/bailian"
	"github.com/0xfig-labs/thinroute/internal/providers/bedrock"
	"github.com/0xfig-labs/thinroute/internal/providers/compatible"
	"github.com/0xfig-labs/thinroute/internal/providers/deepseek"
	"github.com/0xfig-labs/thinroute/internal/providers/fireworks"
	"github.com/0xfig-labs/thinroute/internal/providers/gemini"
	"github.com/0xfig-labs/thinroute/internal/providers/groq"
	"github.com/0xfig-labs/thinroute/internal/providers/kimicode"
	"github.com/0xfig-labs/thinroute/internal/providers/minimax"
	"github.com/0xfig-labs/thinroute/internal/providers/ollama"
	"github.com/0xfig-labs/thinroute/internal/providers/openai"
	"github.com/0xfig-labs/thinroute/internal/providers/opencodego"
	"github.com/0xfig-labs/thinroute/internal/providers/openrouter"
	"github.com/0xfig-labs/thinroute/internal/providers/oracle"
	"github.com/0xfig-labs/thinroute/internal/providers/vertex"
	"github.com/0xfig-labs/thinroute/internal/providers/vllm"
	"github.com/0xfig-labs/thinroute/internal/providers/xai"
	"github.com/0xfig-labs/thinroute/internal/providers/xiaomi"
	"github.com/0xfig-labs/thinroute/internal/providers/zai"
)

// defaultProviderFactory builds the provider factory with every provider type
// the standard gateway ships with.
// DefaultProviderFactory exposes the standard provider registry to local tools.
func DefaultProviderFactory(cfg *config.Config) *providers.ProviderFactory {
	return defaultProviderFactory(cfg)
}

func defaultProviderFactory(cfg *config.Config) *providers.ProviderFactory {
	factory := providers.NewProviderFactory()

	if cfg.Metrics.Enabled {
		factory.SetHooks(observability.NewPrometheusHooks())
	}

	factory.Add(openai.Registration)
	factory.Add(openrouter.Registration)
	factory.Add(azure.Registration)
	factory.Add(bailian.Registration)
	factory.Add(oracle.Registration)
	factory.Add(anthropic.Registration)
	factory.Add(bedrock.Registration)
	factory.Add(deepseek.Registration)
	factory.Add(fireworks.Registration)
	factory.Add(gemini.Registration)
	factory.Add(vertex.Registration)
	factory.Add(groq.Registration)
	factory.Add(kimicode.Registration)
	factory.Add(minimax.Registration)
	factory.Add(ollama.Registration)
	factory.Add(opencodego.Registration)
	factory.Add(vllm.Registration)
	factory.Add(xai.Registration)
	factory.Add(compatible.Registration)
	factory.Add(xiaomi.Registration)
	factory.Add(zai.Registration)

	return factory
}
