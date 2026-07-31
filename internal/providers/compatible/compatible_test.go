package compatible

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/icehugh/thinroute/internal/core"
	"github.com/icehugh/thinroute/internal/providers"
)

func TestProvider_ListModels_UsesBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen/Qwen2.5-Coder-32B-Instruct","object":"model"}]}`))
	}))
	defer server.Close()

	p := New(providers.ProviderConfig{
		Name:    "siliconflow",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}, providers.ProviderOptions{})

	resp, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "qwen/Qwen2.5-Coder-32B-Instruct" {
		t.Fatalf("unexpected models: %+v", resp.Data)
	}
}

func TestProvider_AuthHeader_Bearer(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer server.Close()

	p := New(providers.ProviderConfig{
		Name:    "test",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test-key",
	}, providers.ProviderOptions{})

	_, _ = p.ListModels(context.Background())
	if authHeader != "Bearer sk-test-key" {
		t.Fatalf("Authorization header = %q, want 'Bearer sk-test-key'", authHeader)
	}
}

func TestProvider_ChatCompletion_DoesNotAdaptReasoningParams(t *testing.T) {
	var body strings.Builder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body.Write(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":5}}`))
	}))
	defer server.Close()

	mt := 100
	p := New(providers.ProviderConfig{
		Name:    "test",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}, providers.ProviderOptions{})

	pp := p.(*Provider)
	_, _ = pp.ChatCompletion(context.Background(), &core.ChatRequest{
		Model:     "o3",
		Messages:  []core.Message{{Role: "user", Content: "hi"}},
		MaxTokens: &mt,
	})

	bodyStr := body.String()
	if !strings.Contains(bodyStr, `"max_tokens"`) {
		t.Errorf("body should contain max_tokens, got: %s", bodyStr)
	}
	if strings.Contains(bodyStr, `"max_completion_tokens"`) {
		t.Errorf("body must NOT contain max_completion_tokens, got: %s", bodyStr)
	}
}

func TestProvider_ChatCompletion_StandardPath(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":5,"completion_tokens":10}}`))
	}))
	defer server.Close()

	p := New(providers.ProviderConfig{
		Name:    "test",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}, providers.ProviderOptions{})

	pp := p.(*Provider)
	resp, err := pp.ChatCompletion(context.Background(), &core.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []core.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", path)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestProvider_StreamChatCompletion_StandardPath(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Transfer-Encoding", "chunked")
		_, _ = w.Write([]byte("data: {\"id\":\"chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"streaming\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	p := New(providers.ProviderConfig{
		Name:    "test",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}, providers.ProviderOptions{})

	pp := p.(*Provider)
	rc, err := pp.StreamChatCompletion(context.Background(), &core.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []core.Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("StreamChatCompletion() error = %v", err)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", path)
	}
	rc.Close()
}

func TestProvider_Embeddings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	p := New(providers.ProviderConfig{
		Name:    "test",
		Type:    "compatible",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}, providers.ProviderOptions{})

	_, err := p.Embeddings(context.Background(), &core.EmbeddingRequest{
		Model: "text-embedding-ada-002",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("Embeddings() error = %v", err)
	}
}
