package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0xfig-labs/thinroute/internal/core"
	"github.com/0xfig-labs/thinroute/internal/providers"
	"github.com/0xfig-labs/thinroute/internal/providers/compatible"
	"github.com/0xfig-labs/thinroute/internal/server"
)

// sampleChatResponseJSON is the JSON the mock backend returns for a chat
// completion request. It mirrors the OpenAI chat.completion object shape
// the internal server's ChatCompletion handler parses from the provider.
const sampleChatResponseJSON = `{"id":"chatcmpl-bench","object":"chat.completion","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"Hello! How can I help you today?"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":8,"total_tokens":18}}`

// newMockBackend creates an httptest.Server that returns lightweight
// OpenAI-compatible responses for ChatCompletion and ListModels. The caller
// must close the server.
func newMockBackend() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain and close the request body so the connection stays reusable.
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleChatResponseJSON))

		case r.Method == http.MethodGet && r.URL.Path == "/models":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(core.ModelsResponse{
				Object: "list",
				Data: []core.Model{
					{ID: "gpt-4o-mini", Object: "model", OwnedBy: "mock", Created: 1700000000},
				},
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

// setupThinrouteBenchServer creates a thinroute Server wired to a real Router
// backed by an OpenAI-compatible provider that calls a mock HTTP backend. The
// server starts on a loopback port and is returned alongside its base URL.
//
// Cleanup: the mock backend and server are shut down via b.Cleanup.
func setupThinrouteBenchServer(b *testing.B) (string, *server.Server) {
	b.Helper()

	mockBackend := newMockBackend()
	b.Cleanup(mockBackend.Close)

	// Build a provider factory, register the compatible provider type, and
	// create one instance pointing at the mock backend.
	factory := providers.NewProviderFactory()
	factory.Add(compatible.Registration)

	p, err := factory.Create(providers.ProviderConfig{
		Type:    "compatible",
		Name:    "mock",
		BaseURL: mockBackend.URL,
		APIKey:  "bench-key",
	})
	if err != nil {
		b.Fatalf("factory.Create: %v", err)
	}

	// Seed the model registry and router the way production does.
	registry := providers.NewModelRegistry()
	registry.RegisterProviderWithNameAndType(p, "mock", "compatible")
	if err := registry.Initialize(context.Background()); err != nil {
		b.Fatalf("registry.Initialize: %v", err)
	}

	router, err := providers.NewRouter(registry)
	if err != nil {
		b.Fatalf("NewRouter: %v", err)
	}

	srv := server.New(router, &server.Config{LogOnlyModelInteractions: true})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("net.Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	go func() {
		// StartWithListener blocks until the context is cancelled.
		_ = srv.StartWithListener(ctx, listener)
	}()

	addr := "http://" + listener.Addr().String()

	// Wait for the server to become reachable (poll /health).
	client := &http.Client{Timeout: time.Second}
	for range 20 {
		resp, err := client.Get(addr + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	return addr, srv
}

// BenchmarkThinroute_Health measures the latency of the /health endpoint
// through a fully wired thinroute instance.
func BenchmarkThinroute_Health(b *testing.B) {
	addr, _ := setupThinrouteBenchServer(b)
	client := &http.Client{Timeout: 5 * time.Second}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		resp, err := client.Get(addr + "/health")
		if err != nil {
			b.Fatalf("GET /health: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("GET /health: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	}
}

// BenchmarkThinroute_Chat measures the latency of a non-streaming chat
// completion request through a fully wired thinroute instance with a mock
// backend. The benchmark covers request dispatch to the provider, the
// upstream HTTP round-trip, and response deserialisation back to the client.
func BenchmarkThinroute_Chat(b *testing.B) {
	addr, _ := setupThinrouteBenchServer(b)
	client := &http.Client{Timeout: 10 * time.Second}
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hi"}]}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		resp, err := client.Post(addr+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatalf("POST /v1/chat/completions: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("POST /v1/chat/completions: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	}
}
