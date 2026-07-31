package config

import (
	"fmt"
	"os"
	"strings"
)

// VirtualModelConfig declares one virtual model in config.yaml or the
// VIRTUAL_MODELS env var, so operators can manage redirects, load balancers, and
// access policies as infrastructure-as-code. This is the sole virtual-model
// configuration source.
type VirtualModelConfig struct {
	// Source is the addressable virtual model name (for a redirect/load balancer)
	// or the access selector (for an access policy).
	Source string `yaml:"source" json:"source"`

	// Strategy selects load balancing across multiple targets: "round_robin"
	// (default), "cost", "weighted", "random", "least_used", or "health_aware".
	// Ignored for single-target aliases and access policies.
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`

	// Target is shorthand for a single-target alias, e.g. "openai/gpt-4o". Use
	// Targets instead to load balance across several models.
	Target string `yaml:"target,omitempty" json:"target,omitempty"`

	// Targets are the redirect destinations. One target is a plain alias; several
	// are load balanced across by Strategy.
	Targets []VirtualModelTargetConfig `yaml:"targets,omitempty" json:"targets,omitempty"`

	// UserPaths scopes the entry to specific request user paths. Empty means all.
	UserPaths []string `yaml:"user_paths,omitempty" json:"user_paths,omitempty"`

	// Description is an optional human-readable note.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Enabled toggles the entry. It defaults to true when omitted.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// DisableReasoning strips reasoning controls when this virtual model is requested.
	DisableReasoning bool `yaml:"disable_reasoning,omitempty" json:"disable_reasoning,omitempty"`
}

// VirtualModelTargetConfig is one load-balancing destination. Model may be a bare
// id (with Provider set) or a "provider/model" selector. Weight biases the
// round_robin strategy and defaults to 1.
type VirtualModelTargetConfig struct {
	Provider string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model    string  `yaml:"model" json:"model"`
	Weight   float64 `yaml:"weight,omitempty" json:"weight,omitempty"`
}

const envVirtualModels = "VIRTUAL_MODELS"

// applyVirtualModelsEnv parses the VIRTUAL_MODELS env var — a JSON array of
// virtual model definitions — and merges it over the YAML-declared list. Env
// entries override YAML entries with the same source, consistent with the rest of
// the config pipeline where env always wins.
func applyVirtualModelsEnv(cfg *Config, strict bool) error {
	raw := strings.TrimSpace(os.Getenv(envVirtualModels))
	if raw == "" {
		return nil
	}
	var fromEnv []VirtualModelConfig
	if err := decodeIaCJSON(envVirtualModels, raw, &fromEnv, strict); err != nil {
		return fmt.Errorf("invalid %s: %w", envVirtualModels, err)
	}
	cfg.VirtualModels = mergeByKey(cfg.VirtualModels, fromEnv, func(model VirtualModelConfig) string {
		return canonicalTextKey(model.Source)
	})
	return nil
}
