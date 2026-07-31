package command

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetReturnsErrorOnHTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	cl := &client{baseURL: server.URL, hc: http.Client{}}
	var v any
	if err := cl.get("/control/v1/providers", &v); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestClientPostReturnsErrorOnHTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	cl := &client{baseURL: server.URL, hc: http.Client{}}
	resp, _ := cl.post("/control/v1/providers/test/test", nil)
	body, _ := cl.readBody(resp)
	if body == "" {
		t.Error("expected error body")
	}
}
