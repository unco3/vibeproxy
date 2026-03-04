package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"vibeproxy/internal/config"
	"vibeproxy/internal/secret"
)

// Gateway handles OpenAI-compatible requests and routes them to the appropriate provider.
type Gateway struct {
	models      map[string]string // model prefix → service name
	services    map[string]config.ServiceConfig
	translators map[string]Translator
	secrets     secret.Provider
}

// NewGateway creates a Gateway from config.
func NewGateway(gw config.GatewayConfig, services map[string]config.ServiceConfig, secrets secret.Provider) *Gateway {
	translators := map[string]Translator{
		"openai":    &OpenAITranslator{},
		"anthropic": &AnthropicTranslator{},
	}
	return &Gateway{
		models:      gw.Models,
		services:    services,
		translators: translators,
		secrets:     secrets,
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "only POST is supported")
		return
	}

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
		return
	}
	defer upResp.Body.Close()

	// Handle upstream errors
	if upResp.StatusCode >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upResp.StatusCode)
		io.Copy(w, upResp.Body)
		return
	}

	if req.Stream {
		g.handleStream(w, upResp, translator, req.Model)
	} else {
		g.handleNonStream(w, upResp, translator, req.Model)
	}
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
