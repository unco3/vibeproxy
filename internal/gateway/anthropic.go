package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultAnthropicMaxTokens = 4096
const anthropicAPIVersion = "2023-06-01"

// AnthropicTranslator converts OpenAI-format requests/responses to/from Anthropic format.
type AnthropicTranslator struct{}

// Anthropic request types
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stop        any                `json:"stop_sequences,omitempty"`
}

type anthropicMessage struct {
	Role    string              `json:"role"`
	Content []anthropicContent  `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Anthropic response types
type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []anthropicContent `json:"content"`
	StopReason *string            `json:"stop_reason"`
	Usage      *anthropicUsage    `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (t *AnthropicTranslator) TranslateRequest(req ChatRequest) (io.Reader, http.Header, string, error) {
	ar := anthropicRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
	}

	// Set max_tokens
	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = defaultAnthropicMaxTokens
	}

	// Extract system message
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			ar.System = msg.Content
		} else {
			ar.Messages = append(ar.Messages, anthropicMessage{
				Role: msg.Role,
				Content: []anthropicContent{
					{Type: "text", Text: msg.Content},
				},
			})
		}
	}

	data, err := json.Marshal(ar)
	if err != nil {
		return nil, nil, "", err
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("anthropic-version", anthropicAPIVersion)
	return bytes.NewReader(data), h, "/v1/messages", nil
}

func (t *AnthropicTranslator) TranslateResponse(body io.Reader, model string) (*ChatResponse, error) {
	var ar anthropicResponse
	if err := json.NewDecoder(body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decoding anthropic response: %w", err)
	}

	// Combine content blocks into a single string
	var text string
	for _, c := range ar.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	// Map stop_reason
	var finishReason *string
	if ar.StopReason != nil {
		fr := mapAnthropicStopReason(*ar.StopReason)
		finishReason = &fr
	}

	resp := &ChatResponse{
		ID:      ar.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{
			{
				Index:        0,
				Message:      &Message{Role: "assistant", Content: text},
				FinishReason: finishReason,
			},
		},
	}

	if ar.Usage != nil {
		resp.Usage = &Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
	}

	return resp, nil
}

func (t *AnthropicTranslator) TranslateStream(body io.Reader, model string) StreamTranslator {
	return &anthropicStreamTranslator{
		scanner: newSSEScanner(body),
		model:   model,
	}
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return reason
	}
}
