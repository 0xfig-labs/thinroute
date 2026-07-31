package virtualmodels

import (
	"crypto/rand"
	"math/big"
)

// RandomBalancer picks a target uniformly at random from the viable set.
type RandomBalancer struct{}

// NextTarget returns a uniformly random index into targets. It uses crypto/rand
// to avoid predictability.
func (RandomBalancer) NextTarget(targets []resolvedTarget) int {
	n := int64(len(targets))
	if n <= 1 {
		return 0
	}
	bi, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		// crypto/rand should never fail in practice; fall back to 0.
		return 0
	}
	return int(bi.Int64())
}
