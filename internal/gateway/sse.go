package gateway

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// sseScanner reads SSE events from a stream and yields the data payload of each event.
type sseScanner struct {
	scanner *bufio.Scanner
}

func newSSEScanner(r io.Reader) *sseScanner {
	return &sseScanner{scanner: bufio.NewScanner(r)}
}

// Next returns the data payload of the next SSE event.
// Returns io.EOF when the stream is exhausted.
func (s *sseScanner) Next() ([]byte, error) {
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: ")), nil
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// writeSSEEvent writes a single SSE data event to w.
func writeSSEEvent(w io.Writer, data []byte) error {
	var buf bytes.Buffer
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	_, err := w.Write(buf.Bytes())
	return err
}
