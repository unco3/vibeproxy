package keychain

import "github.com/zalando/go-keyring"

const serviceName = "vibeproxy"

func Set(provider, apiKey string) error {
	return keyring.Set(serviceName, provider, apiKey)
}

func Get(provider string) (string, error) {
	return keyring.Get(serviceName, provider)
}

func Delete(provider string) error {
	return keyring.Delete(serviceName, provider)
}
