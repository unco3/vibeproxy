package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/unco3/vibeproxy/internal/config"
	"github.com/unco3/vibeproxy/internal/secret"
	"github.com/unco3/vibeproxy/internal/token"
)

// maxRequestBodyBytes limits the size of incoming request bodies (10 MB).
const maxRequestBodyBytes = 10 * 1024 * 1024

// Gateway handles OpenAI-compatible requests and routes them to the appropriate provider.
type Gateway struct {
	models      map[string]string // model prefix → service name
	services    map[string]config.ServiceConfig
	translators map[string]Translator
	secrets     secret.Provider
	rateLimiter RateLimiter
	auditLogger AuditLogger
}

// RateLimiter is a subset of policy.RateLimiter used by the gateway.
type RateLimiter interface {
	Allow(service string) bool
}

// AuditLogger is a subset of logging.AuditLogger used by the gateway.
type AuditLogger interface {
	Log(service, method, path string, status int, duration time.Duration, agent string)
}

// NewGateway creates a Gateway from config.
func NewGateway(gw config.GatewayConfig, services map[string]config.ServiceConfig, secrets secret.Provider, rl RateLimiter, audit AuditLogger) *Gateway {
	translators := map[string]Translator{
		"openai":    &OpenAITranslator{},
		"anthropic": &AnthropicTranslator{},
	}
	return &Gateway{
		models:      gw.Models,
		services:    services,
		translators: translators,
		secrets:     secrets,
		rateLimiter: rl,
		auditLogger: audit,
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	agent := r.Header.Get("X-Vibe-Agent")

	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "only POST is supported")
		return
	}

	// Validate dummy token — gateway requires a vp-local-* token
	svcFromToken := g.extractAndValidateToken(r)
	if svcFromToken == "" {
		writeGatewayError(w, http.StatusUnauthorized, "missing or invalid vibeproxy token")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve service from model prefix
	svcName, err := g.resolveService(req.Model)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Rate limit check
	if !g.rateLimiter.Allow(svcName) {
		writeGatewayError(w, http.StatusTooManyRequests, fmt.Sprintf("rate limit exceeded for service %q", svcName))
		g.auditLogger.Log(svcName, r.Method, r.URL.Path, http.StatusTooManyRequests, time.Since(start), agent)
		return
	}

	svc, ok := g.services[svcName]
	if !ok {
		writeGatewayError(w, http.StatusBadRequest, fmt.Sprintf("service %q not configured", svcName))
		return
	}

	translator, ok := g.translators[svcName]
	if !ok {
		writeGatewayError(w, http.StatusBadRequest, fmt.Sprintf("no translator for service %q", svcName))
		return
	}

	// Get real API key
	realKey, err := g.secrets.Get(svcName)
	if err != nil {
		slog.Error("gateway secret lookup failed", "service", svcName, "error", err)
		writeGatewayError(w, http.StatusInternalServerError, "failed to retrieve API key")
		return
	}

	// Translate request
	body, headers, targetPath, err := translator.TranslateRequest(req)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, "request translation failed")
		return
	}

	// Build upstream request
	upstreamURL := strings.TrimRight(svc.Target, "/") + targetPath
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, body)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, "failed to create upstream request")
		return
	}

	// Set headers
	for k, vals := range headers {
		for _, v := range vals {
			upReq.Header.Set(k, v)
		}
	}

	// Inject auth
	if svc.AuthScheme != "" {
		upReq.Header.Set(svc.AuthHeader, svc.AuthScheme+" "+realKey)
	} else {
		upReq.Header.Set(svc.AuthHeader, realKey)
	}

	// Send upstream request
	client := &http.Client{Timeout: 120 * time.Second}
	upResp, err := client.Do(upReq)
	if err != nil {
		slog.Error("gateway upstream error", "service", svcName, "error", err)
		writeGatewayError(w, http.StatusBadGateway, "upstream request failed")
		g.auditLogger.Log(svcName, r.Method, r.URL.Path, http.StatusBadGateway, time.Since(start), agent)
		return
	}
	defer upResp.Body.Close()

	// Handle upstream errors
	if upResp.StatusCode >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upResp.StatusCode)
		io.Copy(w, upResp.Body)
		g.auditLogger.Log(svcName, r.Method, r.URL.Path, upResp.StatusCode, time.Since(start), agent)
		return
	}

	if req.Stream {
		g.handleStream(w, upResp, translator, req.Model)
	} else {
		g.handleNonStream(w, upResp, translator, req.Model)
	}
	g.auditLogger.Log(svcName, r.Method, r.URL.Path, http.StatusOK, time.Since(start), agent)
}

// extractAndValidateToken checks for a valid vp-local-* token in common auth headers.
func (g *Gateway) extractAndValidateToken(r *http.Request) string {
	// Check Authorization: Bearer vp-local-*
	if auth := r.Header.Get("Authorization"); auth != "" {
		tok := strings.TrimPrefix(auth, "Bearer ")
		if token.IsVibeToken(tok) {
			svc, _ := token.ServiceFrom(tok)
			return svc
		}
	}
	// Check x-api-key: vp-local-*
	if key := r.Header.Get("x-api-key"); key != "" {
		if token.IsVibeToken(key) {
			svc, _ := token.ServiceFrom(key)
			return svc
		}
	}
	return ""
}

func (g *Gateway) handleNonStream(w http.ResponseWriter, upResp *http.Response, translator Translator, model string) {
	resp, err := translator.TranslateResponse(upResp.Body, model)
	if err != nil {
		slog.Error("gateway response translation failed", "error", err)
		writeGatewayError(w, http.StatusBadGateway, "response translation failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) handleStream(w http.ResponseWriter, upResp *http.Response, translator Translator, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeGatewayError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	st := translator.TranslateStream(upResp.Body, model)
	for {
		data, err := st.Next()
		if err == io.EOF {
			writeSSEEvent(w, []byte("[DONE]"))
			flusher.Flush()
			return
		}
		if err != nil {
			slog.Error("gateway stream error", "error", err)
			return
		}

		if string(data) == "[DONE]" {
			writeSSEEvent(w, data)
			flusher.Flush()
			return
		}

		writeSSEEvent(w, data)
		flusher.Flush()
	}
}

func (g *Gateway) resolveService(model string) (string, error) {
	for prefix, svcName := range g.models {
		if strings.HasPrefix(model, prefix) {
			return svcName, nil
		}
	}
	return "", fmt.Errorf("unknown model %q: no matching service prefix", model)
}

func writeGatewayError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    nil,
		},
	})
}
