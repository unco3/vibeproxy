package gateway

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestAnthropicStreamTranslator(t *testing.T) {
	// Simulate Anthropic SSE stream
	sseData := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_abc","model":"claude-sonnet-4-6","role":"assistant"}}`,
		"",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		"",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		"",
		`data: {"type":"content_block_stop","index":0}`,
		"",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		"",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	tr := &AnthropicTranslator{}
	st := tr.TranslateStream(strings.NewReader(sseData), "claude-sonnet-4-6")

	// Chunk 1: message_start → role chunk
	data, err := st.Next()
	if err != nil {
		t.Fatalf("chunk 1: %v", err)
	}
	var chunk1 StreamChunk
	json.Unmarshal(data, &chunk1)
	if chunk1.ID != "msg_abc" {
		t.Errorf("chunk 1 id = %q", chunk1.ID)
	}
	if chunk1.Choices[0].Delta.Role != "assistant" {
		t.Errorf("chunk 1 role = %q", chunk1.Choices[0].Delta.Role)
	}

	// Chunk 2: content_block_delta "Hello"
	data, err = st.Next()
	if err != nil {
		t.Fatalf("chunk 2: %v", err)
	}
	var chunk2 StreamChunk
	json.Unmarshal(data, &chunk2)
	if chunk2.Choices[0].Delta.Content != "Hello" {
		t.Errorf("chunk 2 content = %q", chunk2.Choices[0].Delta.Content)
	}

	// Chunk 3: content_block_delta " world"
	data, err = st.Next()
	if err != nil {
		t.Fatalf("chunk 3: %v", err)
	}
	var chunk3 StreamChunk
	json.Unmarshal(data, &chunk3)
	if chunk3.Choices[0].Delta.Content != " world" {
		t.Errorf("chunk 3 content = %q", chunk3.Choices[0].Delta.Content)
	}

	// Chunk 4: message_delta → finish_reason
	data, err = st.Next()
	if err != nil {
		t.Fatalf("chunk 4: %v", err)
	}
	var chunk4 StreamChunk
	json.Unmarshal(data, &chunk4)
	if chunk4.Choices[0].FinishReason == nil || *chunk4.Choices[0].FinishReason != "stop" {
		t.Errorf("chunk 4 finish_reason unexpected")
	}

	// Chunk 5: message_stop → [DONE]
	data, err = st.Next()
	if err != nil {
		t.Fatalf("chunk 5: %v", err)
	}
	if string(data) != "[DONE]" {
		t.Errorf("chunk 5 = %q, want [DONE]", string(data))
	}

	// Should be done
	_, err = st.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF after [DONE], got %v", err)
	}
}
