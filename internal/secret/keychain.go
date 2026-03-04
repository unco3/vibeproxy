package secret

import "github.com/zalando/go-keyring"

const keychainService = "vibeproxy"

// KeychainProvider stores secrets in the OS keychain via go-keyring.
type KeychainProvider struct{}

func (k *KeychainProvider) Get(service string) (string, error) {
	return keyring.Get(keychainService, service)
}

func (k *KeychainProvider) Set(service, key string) error {
	return keyring.Set(keychainService, service, key)
}

func (k *KeychainProvider) Delete(service string) error {
	return keyring.Delete(keychainService, service)
}

func (k *KeychainProvider) Name() string { return "keychain" }
