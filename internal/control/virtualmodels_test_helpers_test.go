package control

import (
	"context"
	"testing"

	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/virtualmodels"
)

type vmTestStore struct {
	items map[string]virtualmodels.VirtualModel
}

func newVMTestStore(items ...virtualmodels.VirtualModel) *vmTestStore {
	s := &vmTestStore{items: map[string]virtualmodels.VirtualModel{}}
	for _, item := range items {
		s.items[item.Source] = item
	}
	return s
}
func (s *vmTestStore) List(context.Context) ([]virtualmodels.VirtualModel, error) {
	out := make([]virtualmodels.VirtualModel, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	return out, nil
}
func (s *vmTestStore) Get(_ context.Context, source string) (*virtualmodels.VirtualModel, error) {
	item, ok := s.items[source]
	if !ok {
		return nil, virtualmodels.ErrNotFound
	}
	return &item, nil
}
func (s *vmTestStore) Upsert(_ context.Context, vm virtualmodels.VirtualModel) error {
	s.items[vm.Source] = vm
	return nil
}
func (s *vmTestStore) Delete(_ context.Context, source string) error {
	delete(s.items, source)
	return nil
}
func (*vmTestStore) Close() error { return nil }

type vmTestCatalog struct {
	providerTypes map[string]string
	models        map[string]core.Model
}

func newVMTestCatalog() *vmTestCatalog {
	return &vmTestCatalog{providerTypes: map[string]string{}, models: map[string]core.Model{}}
}
func (c *vmTestCatalog) add(model, providerType string) {
	c.providerTypes[model] = providerType
	c.models[model] = core.Model{ID: model, Object: "model"}
}
func (c *vmTestCatalog) Supports(model string) bool          { _, ok := c.models[model]; return ok }
func (c *vmTestCatalog) ModelAvailable(model string) bool    { return c.Supports(model) }
func (c *vmTestCatalog) GetProviderType(model string) string { return c.providerTypes[model] }
func (c *vmTestCatalog) LookupModel(model string) (*core.Model, bool) {
	m, ok := c.models[model]
	return &m, ok
}
func (c *vmTestCatalog) ProviderNames() []string { return nil }

func newVMService(t *testing.T, catalog *vmTestCatalog, store virtualmodels.Store, enabled bool) *virtualmodels.Service {
	t.Helper()
	s, err := virtualmodels.NewService(store, catalog, enabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}
