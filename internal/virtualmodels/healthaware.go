package virtualmodels

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/icehugh/thinroute/internal/providers"
)

// HealthAwareBalancer weights targets by circuit breaker state, recency of
// success, and optional cooldown tracker. Targets whose provider has no recent
// successful availability check, or whose provider/model is in cooldown,
// receive weight 0 and are excluded.
type HealthAwareBalancer struct {
	Snapshots []providers.ProviderRuntimeSnapshot
	// Cooldown, when non-nil, is checked per-target to exclude models in cooldown.
	Cooldown *providers.CooldownTracker
}

const healthTimeout = 5 * time.Minute

// NextTarget selects a target weighted by provider health and cooldown state.
func (b *HealthAwareBalancer) NextTarget(targets []resolvedTarget) int {
	if len(targets) == 0 {
		return 0
	}
	if len(targets) == 1 {
		return 0
	}

	health := make(map[string]float64, len(b.Snapshots))
	now := time.Now()
	for _, s := range b.Snapshots {
		health[s.Name] = healthWeight(s, now)
	}

	weights := make([]float64, len(targets))
	total := 0.0
	for i, t := range targets {
		provider := extractProvider(t.qualified)

		// Exclude targets whose provider or model is in cooldown.
		if b.Cooldown != nil {
			if b.Cooldown.Check(providers.CooldownKey{
				ProviderName: provider,
				Scope:        "key",
			}).Active {
				continue
			}
			if b.Cooldown.Check(providers.CooldownKey{
				ProviderName: provider,
				Model:        t.selector.Model,
				Scope:        "model",
			}).Active {
				continue
			}
		}

		w := health[provider]
		if w <= 0 {
			continue
		}
		weights[i] = w
		total += w
	}

	if total <= 0 {
		return 0
	}

	draw := rand.Float64() * total
	running := 0.0
	for i, w := range weights {
		running += w
		if draw < running {
			return i
		}
	}
	return len(targets) - 1
}

func healthWeight(s providers.ProviderRuntimeSnapshot, now time.Time) float64 {
	if s.LastAvailabilityOKAt == nil {
		return 0
	}
	if now.Sub(*s.LastAvailabilityOKAt) < healthTimeout {
		return 5
	}
	return 2
}

func extractProvider(qualified string) string {
	if idx := strings.IndexByte(qualified, '/'); idx >= 0 {
		return qualified[:idx]
	}
	return qualified
}
