package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// OpenAITranslator is a near-passthrough translator for OpenAI-native requests.
type OpenAITranslator struct{}

func (t *OpenAITranslator) TranslateRequest(req ChatRequest) (io.Reader, http.Header, string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, nil, "", err
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return bytes.NewReader(data), h, "/v1/chat/completions", nil
}

func (t *OpenAITranslator) TranslateResponse(body io.Reader, model string) (*ChatResponse, error) {
	var resp ChatResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (t *OpenAITranslator) TranslateStream(body io.Reader, model string) StreamTranslator {
	return &openAIStreamTranslator{body: body}
}

// openAIStreamTranslator passes through OpenAI SSE events as-is.
type openAIStreamTranslator struct {
	body    io.Reader
	scanner *sseScanner
}

func (s *openAIStreamTranslator) Next() ([]byte, error) {
	if s.scanner == nil {
		s.scanner = newSSEScanner(s.body)
	}
	for {
		data, err := s.scanner.Next()
		if err != nil {
			return nil, err
		}
		if string(data) == "[DONE]" {
			return data, nil
		}
		return data, nil
	}
}
