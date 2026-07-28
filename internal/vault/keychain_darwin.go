package vault

import (
	"fmt"
	"os/exec"
	"strings"
)

// The KEK is a generic password item in the login keychain, managed via
// /usr/bin/security so no cgo is needed. Items written by `security` grant
// `security` itself ACL access, so later reads don't prompt.
// Keychain item COORDINATES (service/account names), not credential values.
const (
	keychainService = "dev.stash" // @waiver:backstop-ai/go-standards/backstop.packs.backstop-ai.go-standards.rules.security.go.security.no-hardcoded-credentials:accepted-risk:2026-10-23
	keychainAccount = "kek"       // @waiver:backstop-ai/go-standards/backstop.packs.backstop-ai.go-standards.rules.security.go.security.no-hardcoded-credentials:accepted-risk:2026-10-23
)

func keychainSet(secret string) error {
	// -U updates in place if the item already exists.
	cmd := exec.Command("/usr/bin/security", "add-generic-password",
		"-U", "-s", keychainService, "-a", keychainAccount, "-w", secret)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security add-generic-password: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func keychainGet() (string, error) {
	cmd := exec.Command("/usr/bin/security", "find-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("security find-generic-password: %v (is stash initialized on this machine?)", err)
	}
	return strings.TrimSpace(string(out)), nil
}
