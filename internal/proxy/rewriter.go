package proxy

import (
	"net/http"
	"strings"
)

// ExtractToken is a convenience function that checks the two most common auth patterns.
// For config-driven extraction, use Router.IdentifyRoute() instead.
func ExtractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	return ""
}
