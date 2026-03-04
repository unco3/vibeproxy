package proxy

import (
	"net/http"
	"strings"
)

// Provider defines how to extract and inject auth credentials for a specific API service.
type Provider interface {
	// ExtractToken reads the dummy token from the request based on this provider's auth config.
	ExtractToken(r *http.Request) string
	// InjectKey replaces the auth credential in the request with the real API key.
	InjectKey(r *http.Request, realKey string)
}

// HeaderProvider implements Provider for header-based authentication.
// Covers both "Authorization: Bearer <token>" and "x-api-key: <token>" patterns.
type HeaderProvider struct {
	Header string // e.g. "Authorization", "x-api-key"
	Scheme string // e.g. "Bearer", "" (empty for raw value)
}

func (p *HeaderProvider) ExtractToken(r *http.Request) string {
	val := r.Header.Get(p.Header)
	if val == "" {
		return ""
	}
	if p.Scheme != "" {
		prefix := p.Scheme + " "
		if strings.HasPrefix(val, prefix) {
			return strings.TrimPrefix(val, prefix)
		}
		return ""
	}
	return val
}

func (p *HeaderProvider) InjectKey(r *http.Request, realKey string) {
	if p.Scheme != "" {
		r.Header.Set(p.Header, p.Scheme+" "+realKey)
	} else {
		r.Header.Set(p.Header, realKey)
	}
}
