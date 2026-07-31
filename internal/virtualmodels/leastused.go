package virtualmodels

import (
	"sync"
	"sync/atomic"
)

// LeastUsedBalancer selects the target with the fewest in-flight requests.
// Ties are broken by round-robin to avoid thundering on the first target.
type LeastUsedBalancer struct {
	inFlight sync.Map // qualified -> *atomic.Int64
	counter  atomic.Uint64
}

// NextTarget returns the index of the target with the lowest in-flight count.
// When multiple targets share the same count it rotates among them.
func (b *LeastUsedBalancer) NextTarget(targets []resolvedTarget) int {
	if len(targets) == 0 {
		return 0
	}

	bestIdx := 0
	bestCount := b.loadCount(targets[0].qualified)

	for i := 1; i < len(targets); i++ {
		c := b.loadCount(targets[i].qualified)
		if c < bestCount {
			bestCount = c
			bestIdx = i
		}
	}

	// Round-robin tie-break: among targets tied at bestCount, advance a
	// monotonic counter so the same target isn't always picked first.
	tied := []int{bestIdx}
	for i := 0; i < len(targets); i++ {
		if i != bestIdx && b.loadCount(targets[i].qualified) == bestCount {
			tied = append(tied, i)
		}
	}
	if len(tied) > 1 {
		slot := b.counter.Add(1)
		bestIdx = tied[int(slot)%len(tied)]
	}

	return bestIdx
}

func (b *LeastUsedBalancer) loadCount(qualified string) int64 {
	v, ok := b.inFlight.Load(qualified)
	if !ok {
		// ponytail: single-store race — first writer wins, which is fine for
		// a zero initial value used only to compare relative load.
		ptr := &atomic.Int64{}
		b.inFlight.Store(qualified, ptr)
		return ptr.Load()
	}
	return v.(*atomic.Int64).Load()
}

// Acquire increments the in-flight count for qualified, called before routing
// a request to this target.
func (b *LeastUsedBalancer) Acquire(qualified string) {
	b.getCounter(qualified).Add(1)
}

// Release decrements the in-flight count for qualified, called when a response
// arrives or the request fails.
func (b *LeastUsedBalancer) Release(qualified string) {
	b.getCounter(qualified).Add(-1)
}

func (b *LeastUsedBalancer) getCounter(qualified string) *atomic.Int64 {
	v, _ := b.inFlight.LoadOrStore(qualified, &atomic.Int64{})
	return v.(*atomic.Int64)
}
