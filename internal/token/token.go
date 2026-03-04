package token

import "strings"

const Prefix = "vp-local-"

// ForService returns the deterministic dummy token for a service name.
func ForService(service string) string {
	return Prefix + service
}

// ServiceFrom extracts the service name from a dummy token.
// Returns the service name and true if the token has the expected prefix.
func ServiceFrom(tok string) (string, bool) {
	if !IsVibeToken(tok) {
		return "", false
	}
	return tok[len(Prefix):], true
}

func IsVibeToken(s string) bool {
	return len(s) > len(Prefix) && strings.HasPrefix(s, Prefix)
}
