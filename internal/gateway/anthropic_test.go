package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestAnthropicTranslateRequest(t *testing.T) {
	tr := &AnthropicTranslator{}
	maxTok := 100
	req := ChatRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: &maxTok,
	}

	body, headers, path, err := tr.TranslateRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	if path != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", path)
	}

	if headers.Get("anthropic-version") != anthropicAPIVersion {
		t.Errorf("missing anthropic-version header")
	}

	var ar anthropicRequest
	json.NewDecoder(body).Decode(&ar)

	if ar.System != "You are helpful" {
		t.Errorf("system = %q", ar.System)
	}
	if len(ar.Messages) != 1 {
		t.Fatalf("expected 1 message (no system), got %d", len(ar.Messages))
	}
	if ar.Messages[0].Role != "user" {
		t.Errorf("message role = %q", ar.Messages[0].Role)
	}
	if ar.MaxTokens != 100 {
		t.Errorf("max_tokens = %d, want 100", ar.MaxTokens)
	}
}

func TestAnthropicTranslateRequest_DefaultMaxTokens(t *testing.T) {
	tr := &AnthropicTranslator{}
	req := ChatRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	}

	body, _, _, err := tr.TranslateRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var ar anthropicRequest
	json.NewDecoder(body).Decode(&ar)
	if ar.MaxTokens != defaultAnthropicMaxTokens {
		t.Errorf("max_tokens = %d, want %d", ar.MaxTokens, defaultAnthropicMaxTokens)
	}
}

func TestAnthropicTranslateResponse(t *testing.T) {
	tr := &AnthropicTranslator{}
	stop := "end_turn"
	resp := anthropicResponse{
		ID:    "msg_123",
		Type:  "message",
		Model: "claude-sonnet-4-6",
		Content: []anthropicContent{
			{Type: "text", Text: "Hello world"},
		},
		StopReason: &stop,
		Usage: &anthropicUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	data, _ := json.Marshal(resp)
	chatResp, err := tr.TranslateResponse(bytes.NewReader(data), "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}

	if chatResp.ID != "msg_123" {
		t.Errorf("id = %q", chatResp.ID)
	}
	if chatResp.Object != "chat.completion" {
		t.Errorf("object = %q", chatResp.Object)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chatResp.Choices))
	}
	if chatResp.Choices[0].Message.Content != "Hello world" {
		t.Errorf("content = %q", chatResp.Choices[0].Message.Content)
	}
	if *chatResp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q", *chatResp.Choices[0].FinishReason)
	}
	if chatResp.Usage.PromptTokens != 10 {
		t.Errorf("prompt_tokens = %d", chatResp.Usage.PromptTokens)
	}
	if chatResp.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens = %d", chatResp.Usage.TotalTokens)
	}
}

func TestAnthropicStopReasonMapping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"end_turn", "stop"},
		{"max_tokens", "length"},
		{"stop_sequence", "stop"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := mapAnthropicStopReason(tt.input)
		if got != tt.want {
			t.Errorf("mapAnthropicStopReason(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAnthropicTranslateRequest_Stream(t *testing.T) {
	tr := &AnthropicTranslator{}
	req := ChatRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}

	body, _, _, err := tr.TranslateRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var ar anthropicRequest
	data, _ := io.ReadAll(body)
	json.Unmarshal(data, &ar)
	if !ar.Stream {
		t.Error("expected stream=true in anthropic request")
	}
}
