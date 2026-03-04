package gateway

import (
	"encoding/json"
	"io"
	"time"
)

// anthropicStreamTranslator converts Anthropic SSE events to OpenAI delta chunks.
type anthropicStreamTranslator struct {
	scanner *sseScanner
	model   string
	id      string
	created int64
	done    bool
}

// Anthropic stream event types
type anthropicStreamEvent struct {
	Type string `json:"type"`
}

type anthropicMessageStart struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	} `json:"message"`
}

type anthropicContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type anthropicMessageDelta struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason *string `json:"stop_reason"`
	} `json:"delta"`
	Usage *anthropicUsage `json:"usage"`
}

func (s *anthropicStreamTranslator) Next() ([]byte, error) {
	if s.done {
		return nil, io.EOF
	}

	for {
		data, err := s.scanner.Next()
		if err != nil {
			return nil, err
		}

		// Detect event type
		var event anthropicStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			var ms anthropicMessageStart
			if err := json.Unmarshal(data, &ms); err != nil {
				continue
			}
			s.id = ms.Message.ID
			if ms.Message.Model != "" {
				s.model = ms.Message.Model
			}
			s.created = time.Now().Unix()

			// Emit initial chunk with role
			chunk := StreamChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: s.created,
				Model:   s.model,
				Choices: []Choice{
					{
						Index: 0,
						Delta: &Message{Role: "assistant", Content: ""},
					},
				},
			}
			return json.Marshal(chunk)

		case "content_block_delta":
			var cbd anthropicContentBlockDelta
			if err := json.Unmarshal(data, &cbd); err != nil {
				continue
			}
			if cbd.Delta.Type != "text_delta" {
				continue
			}

			chunk := StreamChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: s.created,
				Model:   s.model,
				Choices: []Choice{
					{
						Index: 0,
						Delta: &Message{Content: cbd.Delta.Text},
					},
				},
			}
			return json.Marshal(chunk)

		case "message_delta":
			var md anthropicMessageDelta
			if err := json.Unmarshal(data, &md); err != nil {
				continue
			}

			var finishReason *string
			if md.Delta.StopReason != nil {
				fr := mapAnthropicStopReason(*md.Delta.StopReason)
				finishReason = &fr
			}

			chunk := StreamChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: s.created,
				Model:   s.model,
				Choices: []Choice{
					{
						Index:        0,
						Delta:        &Message{},
						FinishReason: finishReason,
					},
				},
			}
			return json.Marshal(chunk)

		case "message_stop":
			s.done = true
			return []byte("[DONE]"), nil

		default:
			// Skip ping, content_block_start, content_block_stop, etc.
			continue
		}
	}
}
