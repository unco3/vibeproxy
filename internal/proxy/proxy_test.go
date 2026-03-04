package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"vibeproxy/internal/config"
	"vibeproxy/internal/logging"
	"vibeproxy/internal/policy"
	"vibeproxy/internal/secret"
)

const testDummyToken = "vp-local-testsvc"

// mockSecretProvider is a test helper implementing secret.Provider.
type mockSecretProvider struct {
	keys map[string]string
}

func (m *mockSecretProvider) Get(service string) (string, error) {
	k, ok := m.keys[service]
	if !ok {
		return "", fmt.Errorf("no key for %s", service)
	}
	return k, nil
}
func (m *mockSecretProvider) Set(string, string) error { return nil }
func (m *mockSecretProvider) Delete(string) error      { return nil }
func (m *mockSecretProvider) Name() string              { return "mock" }

// ensure interface compliance
var _ secret.Provider = (*mockSecretProvider)(nil)

func testSetup(t *testing.T, upstream *httptest.Server) (*Router, *policy.Whitelist, *policy.RateLimiter, *logging.AuditLogger) {
	t.Helper()
	upURL, _ := url.Parse(upstream.URL)

	prov := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}
	mock := &mockSecretProvider{keys: map[string]string{"testsvc": "sk-real-secret-key"}}

	router := &Router{
		services: map[string]*Route{
			"testsvc": {
				ServiceName: "testsvc",
				Target:      upURL,
				Provider:    prov,
			},
		},
		providers: []Provider{prov},
		secrets:   mock,
	}

	wl := policy.NewWhitelist(map[string][]string{"testsvc": {"/v1/chat"}})
	rl := policy.NewRateLimiter(map[string]int{"testsvc": 100})
	audit := logging.NewAuditLoggerWriter(io.Discard)

	return router, wl, rl, audit
}

// buildTestHandler creates the full middleware chain with a fake key (no keychain).
func buildTestHandler(router *Router, wl *policy.Whitelist, rl *policy.RateLimiter, audit *logging.AuditLogger, realKey string) http.Handler {
	// Terminal: reverse proxy using route from context
	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routeFrom(r.Context())
		rp := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(route.Target)
				pr.Out.Host = route.Target.Host
				route.Provider.InjectKey(pr.Out, realKey)
			},
		}
		rp.ServeHTTP(w, r)
	})

	// Build chain: same as production but with fake key middleware
	fakeKeyMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := withRealKey(r.Context(), realKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	chain := fakeKeyMiddleware(proxyHandler)
	chain = RateLimitMiddleware(rl, audit)(chain)
	chain = WhitelistMiddleware(wl, audit)(chain)
	chain = AuthMiddleware(router)(chain)
	chain = AuditMiddleware(audit)(chain)

	return chain
}

func TestProxyTokenSwap(t *testing.T) {
	realKey := "sk-real-secret-key"

	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer upstream.Close()

	router, wl, rl, audit := testSetup(t, upstream)
	handler := buildTestHandler(router, wl, rl, audit, realKey)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", bytes.NewBufferString(`{"model":"gpt-4"}`))
	req.Header.Set("Authorization", "Bearer "+testDummyToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	if receivedAuth != "Bearer "+realKey {
		t.Errorf("upstream received auth %q, want %q", receivedAuth, "Bearer "+realKey)
	}
}

func TestProxyRejectsUnknownToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer upstream.Close()

	router, wl, rl, audit := testSetup(t, upstream)
	handler := buildTestHandler(router, wl, rl, audit, "unused")
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer vp-local-unknownsvc")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestProxyRejectsForbiddenPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer upstream.Close()

	router, wl, rl, audit := testSetup(t, upstream)
	handler := buildTestHandler(router, wl, rl, audit, "sk-real")
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/forbidden", nil)
	req.Header.Set("Authorization", "Bearer "+testDummyToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestProxyRateLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer upstream.Close()

	router, wl, _, audit := testSetup(t, upstream)
	rl := policy.NewRateLimiter(map[string]int{"testsvc": 2})
	handler := buildTestHandler(router, wl, rl, audit, "sk-real")
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", nil)
		req.Header.Set("Authorization", "Bearer "+testDummyToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if i < 2 && resp.StatusCode != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
		if i == 2 && resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429, got %d", i, resp.StatusCode)
		}
	}
}

func TestProxyXAPIKeyProvider(t *testing.T) {
	realKey := "sk-ant-real-key"

	var receivedKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("x-api-key")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer upstream.Close()

	upURL, _ := url.Parse(upstream.URL)
	prov := &HeaderProvider{Header: "x-api-key", Scheme: ""}

	router := &Router{
		services: map[string]*Route{
			"anthropic": {ServiceName: "anthropic", Target: upURL, Provider: prov},
		},
		providers: []Provider{prov},
		secrets:   &mockSecretProvider{keys: map[string]string{"anthropic": realKey}},
	}
	wl := policy.NewWhitelist(map[string][]string{"anthropic": {"/v1/messages"}})
	rl := policy.NewRateLimiter(map[string]int{"anthropic": 100})
	audit := logging.NewAuditLoggerWriter(io.Discard)

	handler := buildTestHandler(router, wl, rl, audit, realKey)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/messages", nil)
	req.Header.Set("x-api-key", "vp-local-anthropic")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if receivedKey != realKey {
		t.Errorf("upstream received key %q, want %q", receivedKey, realKey)
	}
}

func TestIsClientDisconnect(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context.Canceled", context.Canceled, true},
		{"net.OpError", &net.OpError{Op: "read", Err: net.ErrClosed}, true},
		{"generic error", io.ErrUnexpectedEOF, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClientDisconnect(tt.err); got != tt.want {
				t.Errorf("isClientDisconnect(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSSEStreamingFlush(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected ResponseWriter to be a Flusher")
		}
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: chunk %d\n\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	router, wl, rl, audit := testSetup(t, upstream)
	handler := buildTestHandler(router, wl, rl, audit, "sk-real")
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer "+testDummyToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var chunks []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			chunks = append(chunks, line)
		}
	}
	if len(chunks) != 3 {
		t.Errorf("expected 3 SSE chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestAgentHeaderInAuditLog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Vibe-Agent"); got != "" {
			t.Errorf("agent header should be stripped before upstream, got %q", got)
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer upstream.Close()

	var logBuf bytes.Buffer
	upURL, _ := url.Parse(upstream.URL)
	prov := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}
	router := &Router{
		services:  map[string]*Route{"testsvc": {ServiceName: "testsvc", Target: upURL, Provider: prov}},
		providers: []Provider{prov},
		secrets:   &mockSecretProvider{keys: map[string]string{"testsvc": "sk-real"}},
	}
	wl := policy.NewWhitelist(map[string][]string{"testsvc": {"/v1/chat"}})
	rl := policy.NewRateLimiter(map[string]int{"testsvc": 100})
	audit := logging.NewAuditLoggerWriter(&logBuf)

	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routeFrom(r.Context())
		agent := agentFrom(r.Context())
		rp := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(route.Target)
				pr.Out.Host = route.Target.Host
				route.Provider.InjectKey(pr.Out, "sk-real")
				pr.Out.Header.Del(AgentHeader)
			},
			ModifyResponse: func(resp *http.Response) error {
				audit.Log(route.ServiceName, r.Method, r.URL.Path, resp.StatusCode, time.Since(startTimeFrom(r.Context())), agent)
				return nil
			},
		}
		rp.ServeHTTP(w, r)
	})

	fakeKey := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := withRealKey(r.Context(), "sk-real")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	chain := fakeKey(proxyHandler)
	chain = RateLimitMiddleware(rl, audit)(chain)
	chain = WhitelistMiddleware(wl, audit)(chain)
	chain = AuthMiddleware(router)(chain)
	chain = AuditMiddleware(audit)(chain)

	proxy := httptest.NewServer(chain)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer "+testDummyToken)
	req.Header.Set("X-Vibe-Agent", "code-agent")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	logStr := logBuf.String()
	if !strings.Contains(logStr, `"agent":"code-agent"`) {
		t.Errorf("audit log should contain agent field, got: %s", logStr)
	}
}

func TestErrorResponseSource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	}))
	defer upstream.Close()

	router, wl, rl, audit := testSetup(t, upstream)
	handler := buildTestHandler(router, wl, rl, audit, "sk-real")
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/forbidden", nil)
	req.Header.Set("Authorization", "Bearer "+testDummyToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var errResp struct {
		Error struct {
			Source string `json:"source"`
			Code   int    `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)

	if errResp.Error.Source != "vibeproxy" {
		t.Errorf("expected source 'vibeproxy', got %q", errResp.Error.Source)
	}
	if errResp.Error.Code != 403 {
		t.Errorf("expected code 403, got %d", errResp.Error.Code)
	}
}

func TestIsStreamingResponse(t *testing.T) {
	tests := []struct {
		name     string
		ct       string
		transfer []string
		want     bool
	}{
		{"SSE", "text/event-stream", nil, true},
		{"chunked JSON", "application/json", []string{"chunked"}, true},
		{"normal JSON", "application/json", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header:           http.Header{"Content-Type": {tt.ct}},
				TransferEncoding: tt.transfer,
			}
			if got := isStreamingResponse(resp); got != tt.want {
				t.Errorf("isStreamingResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpstreamErrorPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limited by upstream"})
	}))
	defer upstream.Close()

	upURL, _ := url.Parse(upstream.URL)
	prov := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}
	router := &Router{
		services:  map[string]*Route{"testsvc": {ServiceName: "testsvc", Target: upURL, Provider: prov}},
		providers: []Provider{prov},
		secrets:   &mockSecretProvider{keys: map[string]string{"testsvc": "sk-real"}},
	}
	wl := policy.NewWhitelist(map[string][]string{"testsvc": {"/v1/chat"}})
	rl := policy.NewRateLimiter(map[string]int{"testsvc": 100})
	audit := logging.NewAuditLoggerWriter(io.Discard)

	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routeFrom(r.Context())
		agent := agentFrom(r.Context())
		start := startTimeFrom(r.Context())
		rp := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(route.Target)
				pr.Out.Host = route.Target.Host
				route.Provider.InjectKey(pr.Out, "sk-real")
				pr.Out.Header.Del(AgentHeader)
			},
			ModifyResponse: func(resp *http.Response) error {
				if resp.StatusCode >= 400 {
					resp.Header.Set("X-Vibeproxy-Error-Source", "upstream")
				}
				audit.Log(route.ServiceName, r.Method, r.URL.Path, resp.StatusCode, time.Since(start), agent)
				return nil
			},
		}
		rp.ServeHTTP(w, r)
	})

	fakeKey := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withRealKey(r.Context(), "sk-real")))
		})
	}
	chain := fakeKey(proxyHandler)
	chain = RateLimitMiddleware(rl, audit)(chain)
	chain = WhitelistMiddleware(wl, audit)(chain)
	chain = AuthMiddleware(router)(chain)
	chain = AuditMiddleware(audit)(chain)

	proxy := httptest.NewServer(chain)
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer "+testDummyToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Vibeproxy-Error-Source"); got != "upstream" {
		t.Errorf("expected X-Vibeproxy-Error-Source 'upstream', got %q", got)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "rate limited by upstream" {
		t.Errorf("expected upstream error body, got %v", body)
	}
}

func TestNewRouterFromConfig(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"openai": {
				Target:       "https://api.openai.com",
				AuthHeader:   "Authorization",
				AuthScheme:   "Bearer",
				AllowedPaths: []string{"/v1/chat/completions"},
			},
			"anthropic": {
				Target:       "https://api.anthropic.com",
				AuthHeader:   "x-api-key",
				AuthScheme:   "",
				AllowedPaths: []string{"/v1/messages"},
			},
		},
	}

	mock := &mockSecretProvider{keys: map[string]string{}}
	router, err := NewRouter(cfg, mock)
	if err != nil {
		t.Fatal(err)
	}

	// Test IdentifyRoute with Bearer token (deterministic: vp-local-openai)
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer vp-local-openai")

	_, route, err := router.IdentifyRoute(req)
	if err != nil {
		t.Fatal(err)
	}
	if route.ServiceName != "openai" {
		t.Errorf("expected openai, got %s", route.ServiceName)
	}

	// Test IdentifyRoute with x-api-key token
	req2, _ := http.NewRequest("POST", "/v1/messages", nil)
	req2.Header.Set("x-api-key", "vp-local-anthropic")

	_, route2, err := router.IdentifyRoute(req2)
	if err != nil {
		t.Fatal(err)
	}
	if route2.ServiceName != "anthropic" {
		t.Errorf("expected anthropic, got %s", route2.ServiceName)
	}
}
