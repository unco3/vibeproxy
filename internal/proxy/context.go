package proxy

import (
	"context"
	"time"
)

type contextKey int

const (
	ctxRoute contextKey = iota
	ctxRealKey
	ctxStartTime
	ctxAgent
	ctxErrorFormatter
)

// AgentHeader is the HTTP header agents use to identify themselves.
const AgentHeader = "X-Vibe-Agent"

func withRoute(ctx context.Context, route *Route) context.Context {
	return context.WithValue(ctx, ctxRoute, route)
}

func routeFrom(ctx context.Context) *Route {
	r, _ := ctx.Value(ctxRoute).(*Route)
	return r
}

func withRealKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxRealKey, key)
}

func realKeyFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxRealKey).(string)
	return s
}

func withStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, ctxStartTime, t)
}

func startTimeFrom(ctx context.Context) time.Time {
	t, _ := ctx.Value(ctxStartTime).(time.Time)
	return t
}

func withAgent(ctx context.Context, agent string) context.Context {
	return context.WithValue(ctx, ctxAgent, agent)
}

func agentFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxAgent).(string)
	return s
}

func withErrorFormatter(ctx context.Context, f ErrorFormatter) context.Context {
	return context.WithValue(ctx, ctxErrorFormatter, f)
}

func errorFormatterFrom(ctx context.Context) ErrorFormatter {
	f, _ := ctx.Value(ctxErrorFormatter).(ErrorFormatter)
	if f == nil {
		return &GenericErrorFormatter{}
	}
	return f
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
