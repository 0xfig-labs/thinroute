package virtualmodels

import (
	"context"
	"sync"
	"testing"

	"github.com/icehugh/thinroute/internal/core"
)

type memoryVMStore struct {
	mu   sync.Mutex
	rows map[string]VirtualModel
}

func newSQLiteVMStore(t *testing.T) Store {
	t.Helper()
	return &memoryVMStore{rows: map[string]VirtualModel{}}
}

func (s *memoryVMStore) List(context.Context) ([]VirtualModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]VirtualModel, 0, len(s.rows))
	for _, vm := range s.rows {
		out = append(out, vm.clone())
	}
	return out, nil
}

func (s *memoryVMStore) Get(_ context.Context, source string) (*VirtualModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vm, ok := s.rows[source]
	if !ok {
		return nil, ErrNotFound
	}
	cloned := vm.clone()
	return &cloned, nil
}

func (s *memoryVMStore) Upsert(_ context.Context, vm VirtualModel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stampUpsert(&vm)
	s.rows[vm.Source] = vm.clone()
	return nil
}

func (s *memoryVMStore) Delete(_ context.Context, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[source]; !ok {
		return ErrNotFound
	}
	delete(s.rows, source)
	return nil
}

func (*memoryVMStore) Close() error { return nil }

type fakeCatalog struct {
	providers []string
	supported map[string]core.Model
	stale     map[string]bool
}

func (c fakeCatalog) Supports(model string) bool {
	_, ok := c.supported[model]
	return ok
}
func (c fakeCatalog) ModelAvailable(model string) bool {
	return c.Supports(model) && !c.stale[model]
}
func (c fakeCatalog) GetProviderType(model string) string {
	if _, ok := c.supported[model]; ok {
		return "openai"
	}
	return ""
}
func (c fakeCatalog) LookupModel(model string) (*core.Model, bool) {
	m, ok := c.supported[model]
	if !ok {
		return nil, false
	}
	return &m, true
}
func (c fakeCatalog) ProviderNames() []string { return c.providers }

func testCatalog() fakeCatalog {
	return fakeCatalog{
		providers: []string{"openai"},
		supported: map[string]core.Model{
			"openai/gpt-4o": {ID: "openai/gpt-4o", Object: "model", OwnedBy: "openai"},
		},
	}
}
