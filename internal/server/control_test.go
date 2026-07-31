package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/icehugh/thinroute/internal/control"
)

func TestControlServerUsesDedicatedPrefixAndToken(t *testing.T) {
	s := NewControlServer(control.NewHandler(nil, nil), "secret")

	for _, test := range []struct {
		path   string
		token  string
		status int
	}{
		{path: "/control/v1/providers", status: http.StatusUnauthorized},
		{path: "/control/v1/providers", token: "secret", status: http.StatusOK},
		{path: "/admin/providers", token: "secret", status: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		if test.token != "" {
			req.Header.Set("Authorization", "Bearer "+test.token)
		}
		rec := httptest.NewRecorder()
		s.echo.ServeHTTP(rec, req)
		if rec.Code != test.status {
			t.Fatalf("%s status = %d, want %d", test.path, rec.Code, test.status)
		}
	}
}
