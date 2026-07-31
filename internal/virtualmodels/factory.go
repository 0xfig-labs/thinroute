package virtualmodels

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xfig-labs/thinroute/config"
)

// Result owns the immutable, configuration-backed virtual-model service.
// Virtual model definitions are never persisted separately from config.yaml.
type Result struct {
	Service *Service
	Store   Store

	closeOnce sync.Once
}

func (r *Result) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {})
	return nil
}

// New builds the virtual-model snapshot solely from declarative configuration.
func New(ctx context.Context, cfg *config.Config, catalog Catalog, declaredProviders []string) (*Result, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	store := readOnlyConfigStore{}
	service, err := NewService(store, catalog, cfg.Models.EnabledByDefault)
	if err != nil {
		return nil, err
	}
	service.SetConfigModels(ConfigModels(cfg.VirtualModels))
	if err := service.Refresh(ctx); err != nil {
		return nil, err
	}
	if err := service.ValidateManagedConfig(declaredProviders); err != nil {
		return nil, err
	}
	return &Result{Service: service, Store: store}, nil
}

// readOnlyConfigStore satisfies the service's narrow snapshot source contract.
// The actual rows live in Service.configModels; mutations are deliberately
// unavailable because config.yaml is the only source of truth.
type readOnlyConfigStore struct{}

func (readOnlyConfigStore) List(context.Context) ([]VirtualModel, error) { return nil, nil }
func (readOnlyConfigStore) Get(context.Context, string) (*VirtualModel, error) {
	return nil, ErrNotFound
}
func (readOnlyConfigStore) Upsert(context.Context, VirtualModel) error {
	return ErrReadOnly
}
func (readOnlyConfigStore) Delete(context.Context, string) error { return ErrReadOnly }
func (readOnlyConfigStore) Close() error                         { return nil }
