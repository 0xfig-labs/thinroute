package virtualmodels

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("virtual model not found")
var ErrReadOnly = errors.New("virtual models are managed by config.yaml")

// Store is retained as the service's narrow snapshot source. Production uses a
// read-only configuration source; mutation methods exist only to keep the
// snapshot engine independently testable.
type Store interface {
	List(ctx context.Context) ([]VirtualModel, error)
	Get(ctx context.Context, source string) (*VirtualModel, error)
	Upsert(ctx context.Context, vm VirtualModel) error
	Delete(ctx context.Context, source string) error
	Close() error
}

func stampUpsert(vm *VirtualModel) {
	now := time.Now().UTC()
	if vm.CreatedAt.IsZero() {
		vm.CreatedAt = now
	}
	vm.UpdatedAt = now
}
