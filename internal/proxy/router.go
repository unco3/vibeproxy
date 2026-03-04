package proxy

import (
	"fmt"
	"net/http"
	"net/url"

	"vibeproxy/internal/config"
	"vibeproxy/internal/secret"
	"vibeproxy/internal/token"
)

// Route holds everything needed to forward a request.
type Route struct {
	ServiceName string
	Target      *url.URL
	Provider    Provider
}

// Router maps deterministic dummy tokens (vp-local-{service}) to routes.
type Router struct {
	services  map[string]*Route // service name → route
	providers []Provider        // unique providers for token scanning
	secrets   secret.Provider
}

func NewRouter(cfg *config.Config, secrets secret.Provider) (*Router, error) {
	r := &Router{services: make(map[string]*Route), secrets: secrets}

	seen := make(map[string]Provider)

	for svcName, svc := range cfg.Services {
		target, err := url.Parse(svc.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid target URL for service %q: %w", svcName, err)
		}

		key := svc.AuthHeader + ":" + svc.AuthScheme
		prov, exists := seen[key]
		if !exists {
			prov = &HeaderProvider{Header: svc.AuthHeader, Scheme: svc.AuthScheme}
			seen[key] = prov
			r.providers = append(r.providers, prov)
		}

		r.services[svcName] = &Route{
			ServiceName: svcName,
			Target:      target,
			Provider:    prov,
		}
	}
	return r, nil
}

// IdentifyRoute scans all known provider auth patterns to extract a dummy token,
// then uses token.ServiceFrom to resolve to a route.
func (r *Router) IdentifyRoute(req *http.Request) (string, *Route, error) {
	for _, prov := range r.providers {
		tok := prov.ExtractToken(req)
		if tok != "" && token.IsVibeToken(tok) {
			svcName, ok := token.ServiceFrom(tok)
			if !ok {
				continue
			}
			route, exists := r.services[svcName]
			if exists {
				return tok, route, nil
			}
		}
	}
	return "", nil, fmt.Errorf("no matching vibeproxy token found")
}

// RealKey fetches the real API key from the secret provider for a route.
func (r *Router) RealKey(route *Route) (string, error) {
	key, err := r.secrets.Get(route.ServiceName)
	if err != nil {
		return "", fmt.Errorf("secret lookup for %q: %w", route.ServiceName, err)
	}
	return key, nil
}
