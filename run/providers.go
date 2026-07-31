package run

import (
	"github.com/icehugh/thinroute/config"
	"github.com/icehugh/thinroute/internal/observability"
	"github.com/icehugh/thinroute/internal/providers"
	"github.com/icehugh/thinroute/internal/providers/anthropic"
	"github.com/icehugh/thinroute/internal/providers/azure"
	"github.com/icehugh/thinroute/internal/providers/bailian"
	"github.com/icehugh/thinroute/internal/providers/bedrock"
	"github.com/icehugh/thinroute/internal/providers/compatible"
	"github.com/icehugh/thinroute/internal/providers/deepseek"
	"github.com/icehugh/thinroute/internal/providers/fireworks"
	"github.com/icehugh/thinroute/internal/providers/gemini"
	"github.com/icehugh/thinroute/internal/providers/groq"
	"github.com/icehugh/thinroute/internal/providers/kimicode"
	"github.com/icehugh/thinroute/internal/providers/minimax"
	"github.com/icehugh/thinroute/internal/providers/ollama"
	"github.com/icehugh/thinroute/internal/providers/openai"
	"github.com/icehugh/thinroute/internal/providers/opencodego"
	"github.com/icehugh/thinroute/internal/providers/openrouter"
	"github.com/icehugh/thinroute/internal/providers/oracle"
	"github.com/icehugh/thinroute/internal/providers/vertex"
	"github.com/icehugh/thinroute/internal/providers/vllm"
	"github.com/icehugh/thinroute/internal/providers/xai"
	"github.com/icehugh/thinroute/internal/providers/xiaomi"
	"github.com/icehugh/thinroute/internal/providers/zai"
)

// defaultProviderFactory builds the provider factory with every provider type
// the standard gateway ships with.
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
