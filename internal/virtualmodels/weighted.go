package virtualmodels

import "math/rand/v2"

// WeightedBalancer selects targets proportionally to their configured Weight.
// A target with weight 2 receives roughly twice the traffic of a target with
// weight 1. This is the same algorithm used internally by the round_robin
// strategy, exposed as a standalone strategy so it can be configured explicitly.
type WeightedBalancer struct{}

// NextTarget picks a target using weighted random selection. It sums all target
// weights, picks a random float in [0, sum), and iterates until the running
// total exceeds the draw.
func (WeightedBalancer) NextTarget(targets []resolvedTarget) int {
	if len(targets) == 0 {
		return 0
	}

	total := 0.0
	for _, t := range targets {
		w := t.weight
		if w <= 0 {
			w = 1
		}
		total += w
	}

	draw := rand.Float64() * total
	running := 0.0
	for i, t := range targets {
		w := t.weight
		if w <= 0 {
			w = 1
		}
		running += w
		if draw <= running {
			return i
		}
	}
	return len(targets) - 1
}
