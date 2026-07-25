//go:build !darwin

package vault

import "fmt"

func keychainSet(secret string) error {
	return fmt.Errorf("only macOS Keychain custody is supported in this MVP (OQ-3 open — headless/Linux fallback deferred)")
}

func keychainGet() (string, error) {
	return "", fmt.Errorf("only macOS Keychain custody is supported in this MVP (OQ-3 open — headless/Linux fallback deferred)")
}
