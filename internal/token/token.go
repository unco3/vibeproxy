package token

import "strings"

const Prefix = "vp-local-"

// ForService returns the deterministic dummy token for a service name.
func ForService(service string) string {
	return Prefix + service
}

// ServiceFrom extracts the service name from a dummy token.
// Returns the service name and true if the token has the expected prefix
// and the service name contains only safe characters (a-z, 0-9, hyphen).
func ServiceFrom(tok string) (string, bool) {
	if !IsVibeToken(tok) {
		return "", false
	}
	svc := tok[len(Prefix):]
	if !isValidServiceName(svc) {
		return "", false
	}
	return svc, true
}

func IsVibeToken(s string) bool {
	return len(s) > len(Prefix) && strings.HasPrefix(s, Prefix)
}

// isValidServiceName checks that a service name contains only lowercase
// alphanumeric characters and hyphens, preventing log injection and
// path traversal via crafted tokens.
func isValidServiceName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}
