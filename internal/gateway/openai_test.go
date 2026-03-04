package gateway

import (
	"encoding/json"
	"testing"
)

func TestOpenAITranslateRequest(t *testing.T) {
	tr := &OpenAITranslator{}
	req := ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	body, headers, path, err := tr.TranslateRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	if path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", path)
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", headers.Get("Content-Type"))
	}

	var decoded ChatRequest
	json.NewDecoder(body).Decode(&decoded)

	if decoded.Model != "gpt-4o" {
		t.Errorf("model = %q", decoded.Model)
	}
	if len(decoded.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(decoded.Messages))
	}
}

func TestOpenAITranslateResponse(t *testing.T) {
	tr := &OpenAITranslator{}
	stop := "stop"
	orig := ChatResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4o",
		Choices: []Choice{
			{Index: 0, Message: &Message{Role: "assistant", Content: "Hi"}, FinishReason: &stop},
		},
		Usage: &Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}

	data, _ := json.Marshal(orig)
	resp, err := tr.TranslateResponse(jsonReader(data), "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("id = %q", resp.ID)
	}
	if resp.Choices[0].Message.Content != "Hi" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 8 {
		t.Errorf("total_tokens = %d", resp.Usage.TotalTokens)
	}
}
