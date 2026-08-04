package providers

import "github.com/0xfig-labs/thinroute/internal/core"

// NativeFileProviderTypes returns the registered provider types that support
// native file operations.
func (r *Router) NativeFileProviderTypes() []string {
	return r.providerTypesSupporting(func(p core.Provider) bool {
		_, ok := p.(core.NativeFileProvider)
		return ok
	})
}

// NativeBatchProviderTypes returns the registered provider types that support
// native batch operations.
func (r *Router) NativeBatchProviderTypes() []string {
	return r.providerTypesSupporting(func(p core.Provider) bool {
		_, ok := p.(core.NativeBatchProvider)
		return ok
	})
}

// NativeResponseProviderTypes returns the registered provider types that
// support native Responses lifecycle operations.
func (r *Router) NativeResponseProviderTypes() []string {
	return r.providerTypesSupporting(func(p core.Provider) bool {
		_, ok := p.(core.NativeResponseLifecycleProvider)
		return ok
	})
}
