package secret

// Provider abstracts secret storage backends (keychain, env vars, etc.).
type Provider interface {
	Get(service string) (string, error)
	Set(service, key string) error
	Delete(service string) error
	Name() string
}
