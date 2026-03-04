package policy

import "strings"

type Whitelist struct {
	paths map[string]map[string]bool // service → set of allowed path prefixes
}

func NewWhitelist(services map[string][]string) *Whitelist {
	w := &Whitelist{paths: make(map[string]map[string]bool)}
	for svc, paths := range services {
		w.paths[svc] = make(map[string]bool, len(paths))
		for _, p := range paths {
			w.paths[svc][p] = true
		}
	}
	return w
}

func (w *Whitelist) Allowed(service, path string) bool {
	allowed, ok := w.paths[service]
	if !ok {
		return false
	}
	for prefix := range allowed {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
