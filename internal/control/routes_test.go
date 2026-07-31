package control

import (
	"sort"
	"testing"

	"github.com/labstack/echo/v5"
)

// TestRegisterRoutes_RegistersExpectedPaths is a smoke test for the         control
// RouteRegistrar plumbing. It mounts the handler on a real echo router and
// verifies that every method+path the route table claims to register is
// actually known to the router after RegisterRoutes returns.
//
// The intent is to catch regressions when handlers are added or renamed
// without updating routes.go (or vice-versa) — including typos and missing
// wires that would otherwise only surface in production traffic.
func TestRegisterRoutes_RegistersExpectedPaths(t *testing.T) {
	h := &Handler{}
	e := echo.New()
	g := e.Group("/control/v1")

	// RegisterRoutes must not panic with a zero-value handler — every endpoint
	// reads its own dependencies inside the handler body, so route mounting
	// itself must remain side-effect-free.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRoutes panicked: %v", r)
		}
	}()
	h.RegisterRoutes(g)

	want := []string{
		"GET /control/v1/runtime/config",
		"POST /control/v1/runtime/refresh",
		"GET /control/v1/cache/overview",

		"GET /control/v1/usage/summary",
		"GET /control/v1/usage/daily",
		"GET /control/v1/usage/models",

		"GET /control/v1/providers",
		"GET /control/v1/providers/cooldown",
		"POST /control/v1/providers/:name/test",
		"POST /control/v1/providers/:name/refresh",

		"GET /control/v1/models",
		"GET /control/v1/models/categories",

		"GET /control/v1/virtual-models",
	}

	registered := make(map[string]struct{})
	for _, route := range e.Router().Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	sort.Strings(want)
	missing := make([]string, 0)
	for _, key := range want {
		if _, ok := registered[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("RegisterRoutes did not register %d route(s):\n  %s", len(missing), missing)
	}

	if got, expected := len(registered), len(want); got != expected {
		extras := make([]string, 0)
		wantSet := make(map[string]struct{}, len(want))
		for _, k := range want {
			wantSet[k] = struct{}{}
		}
		for k := range registered {
			if _, ok := wantSet[k]; !ok {
				extras = append(extras, k)
			}
		}
		sort.Strings(extras)
		t.Fatalf("RegisterRoutes registered %d route(s), want %d; extras: %v", got, expected, extras)
	}
}
