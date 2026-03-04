package secret

import "fmt"

// New creates a Provider for the given backend name.
// Supported backends: "keychain" (default), "env".
func New(backend string) (Provider, error) {
	switch backend {
	case "", "keychain":
		return &KeychainProvider{}, nil
	case "env":
		return &EnvProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown secret backend %q (supported: keychain, env)", backend)
	}
}
