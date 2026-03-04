package gateway

import (
	"io"
	"net/http"
)

// Translator converts between OpenAI-compatible and provider-native formats.
type Translator interface {
	TranslateRequest(req ChatRequest) (body io.Reader, headers http.Header, targetPath string, err error)
	TranslateResponse(body io.Reader, model string) (*ChatResponse, error)
	TranslateStream(body io.Reader, model string) StreamTranslator
}

// StreamTranslator iterates over SSE events, yielding OpenAI-format chunks.
type StreamTranslator interface {
	// Next returns the next SSE data line in OpenAI format.
	// Returns io.EOF when the stream is complete.
	Next() ([]byte, error)
}
