package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/unco3/vibeproxy/internal/config"
	"github.com/unco3/vibeproxy/internal/gateway"
	"github.com/unco3/vibeproxy/internal/logging"
	"github.com/unco3/vibeproxy/internal/policy"
	"github.com/unco3/vibeproxy/internal/secret"
)

type Server struct {
	cfg     *config.Config
	router  *Router
	audit   *logging.AuditLogger
	httpSrv *http.Server
	ln      net.Listener // for Unix domain socket support
}

func NewServer(cfg *config.Config, secrets secret.Provider) (*Server, error) {
	router, err := NewRouter(cfg, secrets)
	if err != nil {
		return nil, err
	}

	wlMap := make(map[string][]string)
	rlMap := make(map[string]int)
	for name, svc := range cfg.Services {
		wlMap[name] = svc.AllowedPaths
		rlMap[name] = svc.RateLimit.RequestsPerMinute
	}

	audit, err := logging.NewAuditLogger()
	if err != nil {
		return nil, fmt.Errorf("audit logger: %w", err)
	}

	whitelist := policy.NewWhitelist(wlMap)
	limiter := policy.NewRateLimiter(rlMap)

	s := &Server{
		cfg:    cfg,
		router: router,
		audit:  audit,
	}

	handler := s.buildChain(router, whitelist, limiter, audit, secrets)

	s.httpSrv = &http.Server{
		Addr:         cfg.Listen,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Timeouts.Read) * time.Second,
		WriteTimeout: time.Duration(cfg.Timeouts.Write) * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s, nil
}

// buildChain assembles the middleware chain in execution order.
func (s *Server) buildChain(router *Router, wl *policy.Whitelist, rl *policy.RateLimiter, audit *logging.AuditLogger, secrets secret.Provider) http.Handler {
	// Terminal handler: reverse proxy with token swap
	proxyHandler := s.reverseProxyHandler(router, secrets)

	// Proxy chain: Auth → Whitelist → RateLimit → ReverseProxy
	proxyChain := http.Handler(proxyHandler)
	proxyChain = RateLimitMiddleware(rl, audit)(proxyChain)
	proxyChain = WhitelistMiddleware(wl, audit)(proxyChain)
	proxyChain = AuthMiddleware(router)(proxyChain)

	var handler http.Handler = proxyChain

	// If gateway is enabled, gateway paths go to Gateway handler (has its own
	// token validation, rate limiting, and audit logging). All other paths go
	// through the normal proxy middleware chain.
	if s.cfg.Gateway.Enabled {
		gw := gateway.NewGateway(s.cfg.Gateway, s.cfg.Services, secrets, rl, audit)
		gwPaths := make(map[string]bool, len(s.cfg.Gateway.Paths))
		for _, p := range s.cfg.Gateway.Paths {
			gwPaths[p] = true
		}
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gwPaths[r.URL.Path] {
				gw.ServeHTTP(w, r)
			} else {
				proxyChain.ServeHTTP(w, r)
			}
		})
	}

	// Outer chain: CORS → Audit → BodyLimit → (gateway dispatch or proxy chain)
	chain := BodyLimitMiddleware()(handler)
	chain = AuditMiddleware(audit)(chain)
	chain = corsMiddleware(chain, s.cfg.CORS)

	return chain
}

// reverseProxyHandler is the terminal handler that performs the actual proxy forwarding.
// It resolves the real API key inline to minimize the key's lifetime in memory.
func (s *Server) reverseProxyHandler(router *Router, secrets secret.Provider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routeFrom(r.Context())
		start := startTimeFrom(r.Context())
		agent := agentFrom(r.Context())

		// Resolve real key inline — never stored in context
		realKey, err := secrets.Get(route.ServiceName)
		if err != nil {
			slog.Error("secret lookup failed", "service", route.ServiceName, "error", err)
			errorFormatterFrom(r.Context()).WriteError(w, http.StatusInternalServerError, "failed to retrieve API key")
			return
		}

		transport := &http.Transport{
			ResponseHeaderTimeout: time.Duration(s.cfg.Timeouts.Upstream) * time.Second,
		}
		rp := &httputil.ReverseProxy{
			Transport: transport,
			// Flush immediately for SSE/streaming — LLM APIs stream via text/event-stream
			// and buffering causes visible latency for the agent.
			FlushInterval: -1, // flush every write
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(route.Target)
				pr.Out.Host = route.Target.Host
				route.Provider.InjectKey(pr.Out, realKey)
				// Strip agent identity header before forwarding to upstream
				pr.Out.Header.Del(AgentHeader)
			},
			ModifyResponse: func(resp *http.Response) error {
				// Disable WriteTimeout for streaming responses so long-lived SSE
				// connections aren't killed by the server's global timeout.
				if isStreamingResponse(resp) {
					rc := http.NewResponseController(w)
					rc.SetWriteDeadline(time.Time{}) // no deadline
				}

				// Tag upstream error responses so agents can distinguish
				// "proxy rejected my request" vs "upstream API returned an error".
				if resp.StatusCode >= 400 {
					resp.Header.Set("X-Vibeproxy-Error-Source", "upstream")
					slog.Warn("upstream error response",
						"service", route.ServiceName,
						"method", r.Method,
						"path", r.URL.Path,
						"status", resp.StatusCode,
						"agent", agent,
					)
				}

				s.audit.Log(route.ServiceName, r.Method, r.URL.Path, resp.StatusCode, time.Since(start), agent)
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, proxyErr error) {
				duration := time.Since(start)

				if isClientDisconnect(proxyErr) {
					slog.Debug("client disconnected",
						"service", route.ServiceName,
						"method", r.Method,
						"path", r.URL.Path,
						"agent", agent,
						"duration", duration,
					)
					s.audit.Log(route.ServiceName, r.Method, r.URL.Path, 499, duration, agent)
					return
				}

				slog.Error("proxy upstream error",
					"service", route.ServiceName,
					"method", r.Method,
					"path", r.URL.Path,
					"agent", agent,
					"error", proxyErr,
					"duration", duration,
				)
				detail := "connection error"
				if errors.Is(proxyErr, context.DeadlineExceeded) {
					detail = "upstream timeout"
				}
				errorFormatterFrom(r.Context()).WriteProxyError(w, http.StatusBadGateway, "upstream error", detail)
				s.audit.Log(route.ServiceName, r.Method, r.URL.Path, http.StatusBadGateway, duration, agent)
			},
		}
		rp.ServeHTTP(w, r)
	})
}

// isStreamingResponse detects SSE or chunked streaming responses from LLM APIs.
func isStreamingResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return true
	}
	for _, v := range resp.TransferEncoding {
		if v == "chunked" {
			return true
		}
	}
	return false
}

func (s *Server) ListenAndServe() error {
	// Unix domain socket mode
	if s.cfg.ListenUnix != "" {
		return s.listenUnix()
	}

	// TCP mode — ensure localhost-only binding
	host, _, err := net.SplitHostPort(s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("refusing to bind to non-localhost address %q", host)
	}

	slog.Info("VibeProxy listening", "addr", s.cfg.Listen)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(timeout time.Duration) error {
	ctx, cancel := contextWithTimeout(timeout)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// isClientDisconnect returns true if the error indicates the client closed the connection.
func isClientDisconnect(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

