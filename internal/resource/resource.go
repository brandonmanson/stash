// Package resource defines the single Stash primitive: everything is a
// resource — {key, value, type, metadata, tags, policy} — addressed by a
// hierarchical dot-namespace (DD-2).
package resource

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Resource is the stored record. Value is held decrypted here; the store is
// responsible for encrypting it at rest.
type Resource struct {
	Key      string            `json:"key"`
	Type     string            `json:"type"`
	Value    []byte            `json:"value,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Policy   map[string]string `json:"policy,omitempty"`
	// Description is free text written to be found by future-you — it is
	// non-secret by construction and is the primary input to recall.
	Description string `json:"description,omitempty"`
	// Reserved marks a declared leaf with no value yet: the key and type are
	// claimed (and protected by the collision rules) before custody exists.
	Reserved  bool      `json:"reserved,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Entry is a metadata-only listing row (no value, no decryption required).
type Entry struct {
	Key         string    `json:"key"`
	Type        string    `json:"type"`
	Tags        []string  `json:"tags,omitempty"`
	Description string    `json:"description,omitempty"`
	Reserved    bool      `json:"reserved,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Built-in types (OQ-7: curated set plus freeform escape hatch).
const (
	TypeCredential = "credential"
	// These consts are resource TYPE NAMES in a secrets store, not secrets.
	TypePassword    = "password" // @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-hardcoded-credentials:accepted-risk:2026-10-23
	TypeToken       = "token"    // @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-hardcoded-credentials:accepted-risk:2026-10-23
	TypeCertificate = "certificate"
	TypeNote        = "note"
	TypeLink        = "link"
	TypeEndpoint    = "endpoint"
	TypeDate        = "date"
	TypeBlob        = "blob"
)

// IsSecret reports whether values of this type should be masked by default.
func IsSecret(typ string) bool {
	switch typ {
	case TypeCredential, TypePassword, TypeToken, TypeCertificate:
		return true
	default:
		return false
	}
}

// typeForSegment maps a key segment to a default type. Namespaces are
// entity-first in practice (`resend.credentials.key`, `jason.birthday`), so
// a type-ish segment can appear anywhere in the key, not just first.
func typeForSegment(seg string) (string, bool) {
	switch seg {
	case "credentials", "credential", "creds":
		return TypeCredential, true
	case "passwords", "password":
		return TypePassword, true
	case "tokens", "token":
		return TypeToken, true
	case "certs", "cert", "certificates", "certificate":
		return TypeCertificate, true
	case "links", "link":
		return TypeLink, true
	case "endpoints", "endpoint":
		return TypeEndpoint, true
	case "notes", "note":
		return TypeNote, true
	case "dates", "date", "birthdays", "birthday":
		return TypeDate, true
	default:
		return "", false
	}
}

// InferType returns the default type for a key: the first segment, scanning
// left to right, that names a known type. `resend.credentials.key` is a
// credential; `jason.birthday` is a date.
func InferType(key string) string {
	for _, seg := range strings.Split(key, ".") {
		if t, ok := typeForSegment(seg); ok {
			return t
		}
	}
	return TypeNote
}

var segmentRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidateKey enforces dot-namespace key syntax: lowercase alphanumeric
// segments (plus - and _) separated by single dots.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	for _, seg := range strings.Split(key, ".") {
		if !segmentRe.MatchString(seg) {
			return fmt.Errorf("invalid key %q: segments must match %s, separated by dots", key, segmentRe)
		}
	}
	return nil
}

// Ancestors returns every namespace prefix of key, nearest last:
// "a.b.c" -> ["a", "a.b"].
func Ancestors(key string) []string {
	var out []string
	for i, r := range key {
		if r == '.' {
			out = append(out, key[:i])
		}
	}
	return out
}
