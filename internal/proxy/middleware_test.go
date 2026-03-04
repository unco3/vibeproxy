package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"vibeproxy/internal/logging"
	"vibeproxy/internal/policy"
)

func timeNow() time.Time { return time.Now() }

func dummyRoute() *Route {
	target, _ := url.Parse("http://localhost:9999")
	return &Route{
		ServiceName: "testsvc",
		Target:      target,
		Provider:    &HeaderProvider{Header: "Authorization", Scheme: "Bearer"},
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	target, _ := url.Parse("http://localhost:9999")
	router := &Router{
		services: map[string]*Route{
			"testsvc": {
				ServiceName: "testsvc",
				Target:      target,
				Provider:    &HeaderProvider{Header: "Authorization", Scheme: "Bearer"},
			},
		},
		providers: []Provider{&HeaderProvider{Header: "Authorization", Scheme: "Bearer"}},
		secrets:   nil,
	}

	var gotRoute *Route
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRoute = routeFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(router)(inner)

	req := httptest.NewRequest("POST", "/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer vp-local-testsvc")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotRoute == nil || gotRoute.ServiceName != "testsvc" {
		t.Error("route should be set in context")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	router := &Router{
		services:  map[string]*Route{},
		providers: []Provider{&HeaderProvider{Header: "Authorization", Scheme: "Bearer"}},
		secrets:   nil,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := AuthMiddleware(router)(inner)

	req := httptest.NewRequest("POST", "/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer vp-local-unknownsvc")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestWhitelistMiddleware_Allowed(t *testing.T) {
	audit := logging.NewAuditLoggerWriter(io.Discard)
	wl := policy.NewWhitelist(map[string][]string{"testsvc": {"/v1/chat"}})

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := WhitelistMiddleware(wl, audit)(inner)

	req := httptest.NewRequest("POST", "/v1/chat", nil)
	ctx := withRoute(req.Context(), dummyRoute())
	ctx = withStartTime(ctx, timeNow())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should have been called")
	}
}

func TestWhitelistMiddleware_Forbidden(t *testing.T) {
	audit := logging.NewAuditLoggerWriter(io.Discard)
	wl := policy.NewWhitelist(map[string][]string{"testsvc": {"/v1/chat"}})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := WhitelistMiddleware(wl, audit)(inner)

	req := httptest.NewRequest("POST", "/v1/forbidden", nil)
	ctx := withRoute(req.Context(), dummyRoute())
	ctx = withStartTime(ctx, timeNow())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRateLimitMiddleware_Exceeded(t *testing.T) {
	audit := logging.NewAuditLoggerWriter(io.Discard)
	limiter := policy.NewRateLimiter(map[string]int{"testsvc": 1})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(limiter, audit)(inner)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/chat", nil)
		ctx := withRoute(req.Context(), dummyRoute())
		ctx = withStartTime(ctx, timeNow())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if i == 0 && rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429, got %d", i, rec.Code)
		}
	}
}
