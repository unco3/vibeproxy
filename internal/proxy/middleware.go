package proxy

import (
	"fmt"
	"net/http"
	"time"

	"github.com/unco3/vibeproxy/internal/logging"
	"github.com/unco3/vibeproxy/internal/policy"
)

// AuditMiddleware records the start time and extracts the agent identity header.
func AuditMiddleware(audit *logging.AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := withStartTime(r.Context(), time.Now())
			if agent := r.Header.Get(AgentHeader); agent != "" {
				ctx = withAgent(ctx, agent)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthMiddleware uses the Router to identify the route from request headers.
// Sets route and error formatter in context for downstream middlewares.
func AuthMiddleware(router *Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, route, err := router.IdentifyRoute(r)
			if err != nil {
				errorFormatterFrom(r.Context()).WriteError(w, http.StatusUnauthorized, "missing or invalid vibeproxy token")
				return
			}

			ctx := withRoute(r.Context(), route)
			ctx = withErrorFormatter(ctx, FormatterForService(route.ServiceName))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WhitelistMiddleware checks the request path against allowed paths for the service.
func WhitelistMiddleware(wl *policy.Whitelist, audit *logging.AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := routeFrom(r.Context())
			if route == nil {
				errorFormatterFrom(r.Context()).WriteError(w, http.StatusInternalServerError, "internal routing error")
				return
			}

			if !wl.Allowed(route.ServiceName, r.URL.Path) {
				errorFormatterFrom(r.Context()).WriteError(w, http.StatusForbidden, fmt.Sprintf("path %q not allowed for service %q", r.URL.Path, route.ServiceName))
				audit.Log(route.ServiceName, r.Method, r.URL.Path, http.StatusForbidden, time.Since(startTimeFrom(r.Context())), agentFrom(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware enforces per-service rate limits.
func RateLimitMiddleware(limiter *policy.RateLimiter, audit *logging.AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := routeFrom(r.Context())
			if route == nil {
				errorFormatterFrom(r.Context()).WriteError(w, http.StatusInternalServerError, "internal routing error")
				return
			}

			if !limiter.Allow(route.ServiceName) {
				errorFormatterFrom(r.Context()).WriteError(w, http.StatusTooManyRequests, fmt.Sprintf("rate limit exceeded for service %q", route.ServiceName))
				audit.Log(route.ServiceName, r.Method, r.URL.Path, http.StatusTooManyRequests, time.Since(startTimeFrom(r.Context())), agentFrom(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimitMiddleware limits the size of request bodies to prevent memory exhaustion.
func BodyLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10 MB
			}
			next.ServeHTTP(w, r)
		})
	}
}
