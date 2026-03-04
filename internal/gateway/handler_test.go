package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unco3/vibeproxy/internal/config"
	"github.com/unco3/vibeproxy/internal/secret"
)

type mockSecrets struct {
	keys map[string]string
}

func (m *mockSecrets) Get(svc string) (string, error) {
	k, ok := m.keys[svc]
	if !ok {
		return "", fmt.Errorf("no key for %s", svc)
	}
	return k, nil
}
func (m *mockSecrets) Set(string, string) error { return nil }
func (m *mockSecrets) Delete(string) error      { return nil }
func (m *mockSecrets) Name() string              { return "mock" }

var _ secret.Provider = (*mockSecrets)(nil)

func TestGatewayNonStream(t *testing.T) {
	// Mock OpenAI upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test-key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}

		stop := "stop"
		json.NewEncoder(w).Encode(ChatResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Model:   "gpt-4o",
			Choices: []Choice{{Index: 0, Message: &Message{Role: "assistant", Content: "Hi there"}, FinishReason: &stop}},
		})
	}))
	defer upstream.Close()

	gw := NewGateway(
		config.GatewayConfig{
			Enabled: true,
			Models:  map[string]string{"gpt-": "openai"},
		},
		map[string]config.ServiceConfig{
			"openai": {
				Target:     upstream.URL,
				AuthHeader: "Authorization",
				AuthScheme: "Bearer",
			},
		},
		&mockSecrets{keys: map[string]string{"openai": "sk-test-key"}},
	)

	body, _ := json.Marshal(ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp ChatResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Choices[0].Message.Content != "Hi there" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
}

func TestGatewayAnthropicTranslation(t *testing.T) {
	// Mock Anthropic upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-key" {
			t.Errorf("auth = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Errorf("missing anthropic-version header")
		}

		// Verify request was translated
		var ar anthropicRequest
		json.NewDecoder(r.Body).Decode(&ar)
		if ar.System != "You are helpful" {
			t.Errorf("system = %q", ar.System)
		}

		stop := "end_turn"
		json.NewEncoder(w).Encode(anthropicResponse{
			ID:      "msg_test",
			Type:    "message",
			Model:   "claude-sonnet-4-6",
			Content: []anthropicContent{{Type: "text", Text: "Hello from Claude"}},
			StopReason: &stop,
			Usage:   &anthropicUsage{InputTokens: 10, OutputTokens: 5},
		})
	}))
	defer upstream.Close()

	gw := NewGateway(
		config.GatewayConfig{
			Enabled: true,
			Models:  map[string]string{"claude-": "anthropic"},
		},
		map[string]config.ServiceConfig{
			"anthropic": {
				Target:     upstream.URL,
				AuthHeader: "x-api-key",
				AuthScheme: "",
			},
		},
		&mockSecrets{keys: map[string]string{"anthropic": "sk-ant-key"}},
	)

	body, _ := json.Marshal(ChatRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp ChatResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Object != "chat.completion" {
		t.Errorf("object = %q", resp.Object)
	}
	if resp.Choices[0].Message.Content != "Hello from Claude" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
	if *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q", *resp.Choices[0].FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestGatewayUnknownModel(t *testing.T) {
	gw := NewGateway(
		config.GatewayConfig{Enabled: true, Models: map[string]string{"gpt-": "openai"}},
		map[string]config.ServiceConfig{},
		&mockSecrets{keys: map[string]string{}},
	)

	body, _ := json.Marshal(ChatRequest{
		Model:    "unknown-model",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGatewayStream(t *testing.T) {
	// Mock Anthropic SSE upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_s","model":"claude-sonnet-4-6"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`data: {"type":"message_stop"}`,
		}
		for _, e := range events {
			fmt.Fprintln(w, e)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	gw := NewGateway(
		config.GatewayConfig{
			Enabled: true,
			Models:  map[string]string{"claude-": "anthropic"},
		},
		map[string]config.ServiceConfig{
			"anthropic": {Target: upstream.URL, AuthHeader: "x-api-key"},
		},
		&mockSecrets{keys: map[string]string{"anthropic": "key"}},
	)

	body, _ := json.Marshal(ChatRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	// Parse SSE events from response
	respBody := rec.Body.String()
	var chunks []string
	for _, line := range strings.Split(respBody, "\n") {
		if strings.HasPrefix(line, "data: ") {
			chunks = append(chunks, strings.TrimPrefix(line, "data: "))
		}
	}

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d: %v", len(chunks), chunks)
	}

	// Last chunk should be [DONE]
	last := chunks[len(chunks)-1]
	if last != "[DONE]" {
		t.Errorf("last chunk = %q, want [DONE]", last)
	}

	// First chunk should have role
	var first StreamChunk
	json.Unmarshal([]byte(chunks[0]), &first)
	if first.Choices[0].Delta.Role != "assistant" {
		t.Errorf("first chunk role = %q", first.Choices[0].Delta.Role)
	}
}

func TestGatewayMethodNotAllowed(t *testing.T) {
	gw := NewGateway(
		config.GatewayConfig{Enabled: true, Models: map[string]string{}},
		map[string]config.ServiceConfig{},
		&mockSecrets{keys: map[string]string{}},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 405 {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestGatewayUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer upstream.Close()

	gw := NewGateway(
		config.GatewayConfig{Enabled: true, Models: map[string]string{"gpt-": "openai"}},
		map[string]config.ServiceConfig{
			"openai": {Target: upstream.URL, AuthHeader: "Authorization", AuthScheme: "Bearer"},
		},
		&mockSecrets{keys: map[string]string{"openai": "key"}},
	)

	body, _ := json.Marshal(ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 429 {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}
