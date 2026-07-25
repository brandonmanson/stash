// Package vault owns key custody and value encryption (DD-5).
//
// Layout: a random 256-bit KEK lives in the macOS Keychain; a random 256-bit
// data key, wrapped by the KEK with AES-256-GCM, lives in keys.json next to
// the store. Values are encrypted per-resource with the data key, with the
// resource key bound in as AAD so a ciphertext cannot be swapped between
// resources. (OQ-2/OQ-3 leans, adopted provisionally for the MVP — see
// DECISIONS.md.)
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const keysFile = "keys.json" // a filename, not a credential — @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-hardcoded-credentials:accepted-risk:2026-10-23

type keysConfig struct {
	Version        int    `json:"version"`
	Method         string `json:"method"` // "keychain" is the only MVP method
	WrappedDataKey string `json:"wrapped_data_key"`
}

// Vault holds the unwrapped data key for a session.
type Vault struct {
	dataKey []byte
}

// Init creates the key material for a new stash home: KEK into the OS
// keychain, wrapped data key into keys.json. Fails if keys.json exists.
func Init(home string) error {
	path := filepath.Join(home, keysFile)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("stash is already initialized (%s exists)", path)
	}
	kek := make([]byte, 32)
	dataKey := make([]byte, 32)
	if _, err := rand.Read(kek); err != nil {
		return fmt.Errorf("generating master key: %w", err)
	}
	if _, err := rand.Read(dataKey); err != nil {
		return fmt.Errorf("generating data key: %w", err)
	}
	if err := keychainSet(hex.EncodeToString(kek)); err != nil {
		return fmt.Errorf("storing master key in keychain: %w", err)
	}
	wrapped, err := seal(kek, dataKey, []byte("stash.data-key.v1"))
	if err != nil {
		return fmt.Errorf("wrapping data key: %w", err)
	}
	cfg := keysConfig{Version: 1, Method: "keychain", WrappedDataKey: hex.EncodeToString(wrapped)}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding key config: %w", err)
	}
	return os.WriteFile(path, raw, 0o600)
}

// Open unlocks the vault: fetch KEK from the keychain, unwrap the data key.
func Open(home string) (*Vault, error) {
	raw, err := os.ReadFile(filepath.Join(home, keysFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("stash is not initialized — run `stash init`")
		}
		return nil, fmt.Errorf("reading key config: %w", err)
	}
	var cfg keysConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", keysFile, err)
	}
	if cfg.Method != "keychain" {
		return nil, fmt.Errorf("unsupported key custody method %q", cfg.Method)
	}
	kekHex, err := keychainGet()
	if err != nil {
		return nil, fmt.Errorf("fetching master key from keychain: %w", err)
	}
	kek, err := hex.DecodeString(kekHex)
	if err != nil {
		return nil, fmt.Errorf("keychain item is corrupt: %w", err)
	}
	wrapped, err := hex.DecodeString(cfg.WrappedDataKey)
	if err != nil {
		return nil, fmt.Errorf("keys.json is corrupt: %w", err)
	}
	dataKey, err := open(kek, wrapped, []byte("stash.data-key.v1"))
	if err != nil {
		return nil, fmt.Errorf("unwrapping data key (keychain/keys.json mismatch?): %w", err)
	}
	return &Vault{dataKey: dataKey}, nil
}

// Encrypt seals a resource value, binding the resource key as AAD.
func (v *Vault) Encrypt(resourceKey string, plaintext []byte) ([]byte, error) {
	return seal(v.dataKey, plaintext, []byte(resourceKey))
}

// Decrypt opens a resource value previously sealed for resourceKey.
func (v *Vault) Decrypt(resourceKey string, blob []byte) ([]byte, error) {
	pt, err := open(v.dataKey, blob, []byte(resourceKey))
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: %w", resourceKey, err)
	}
	return pt, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating AEAD: %w", err)
	}
	return aead, nil
}

// seal returns nonce||ciphertext.
func seal(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := gcm(key)
	if err != nil {
		return nil, fmt.Errorf("sealing: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, blob, aad []byte) ([]byte, error) {
	aead, err := gcm(key)
	if err != nil {
		return nil, fmt.Errorf("opening: %w", err)
	}
	if len(blob) < aead.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return aead.Open(nil, blob[:aead.NonceSize()], blob[aead.NonceSize():], aad)
}
