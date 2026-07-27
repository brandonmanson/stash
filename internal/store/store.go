// Package store persists resources. Store is an interface so the MVP's
// CLI-direct wiring can later be fronted by stashd over IPC without touching
// callers (the DD-4 seam — see DECISIONS.md).
package store

import (
	"fmt"

	"github.com/brandonmanson/stash/internal/resource"
)

// NotFoundError reports a key with no stored resource.
type NotFoundError struct{ Key string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("resource not found: %s", e.Key) }

// CollisionError enforces the leaf-XOR-namespace rule (OQ-6): a node is
// either a value or a namespace, never both.
type CollisionError struct {
	Key       string
	Conflict  string
	KeyIsLeaf bool // true: Key exists as leaf blocking namespace use; false: Conflict blocks Key
}

func (e *CollisionError) Error() string {
	if e.KeyIsLeaf {
		return fmt.Sprintf("%q already holds a value; it cannot also be a namespace (conflicts with %q)", e.Key, e.Conflict)
	}
	return fmt.Sprintf("%q is a namespace (contains %q); it cannot also hold a value", e.Key, e.Conflict)
}

// Store is the persistence contract. Values arrive/leave encrypted — the
// caller (CLI now, stashd later) owns the vault.
//
// Reservations resolve lazily: a reserved leaf is unresolved intent, and the
// first concrete action decides it. Putting AT it fills it as a leaf; putting
// (or reserving) UNDER it dissolves it into a namespace — Put/Reserve return
// the dissolved keys so the caller can surface a notice. Dissolution only
// flows downward: a put at a coarse key never destroys a deeper reservation.
type Store interface {
	// Put inserts or updates a resource. Value must already be encrypted.
	// Putting to a reserved key fills the reservation.
	Put(res resource.Resource) (dissolved []string, err error)
	// Reserve claims a key (and type) with no value. Idempotent on an
	// already-reserved key; errors if the key already holds a value.
	Reserve(res resource.Resource) (dissolved []string, err error)
	// Get returns the resource with its encrypted value.
	Get(key string) (resource.Resource, error)
	// List returns metadata-only entries for the given namespace prefix
	// ("" for everything). Prefix "a.b" matches "a.b" and "a.b.*".
	List(prefix string) ([]resource.Entry, error)
	// Search matches q case-insensitively against key, type, tags,
	// description, and metadata (values are encrypted and not searchable).
	Search(q string) ([]resource.Entry, error)
	// Delete removes a resource and any embedding derived from it.
	Delete(key string) error
	// PutEmbedding stores or replaces the embedding for a key.
	PutEmbedding(e Embedding) error
	// ListEmbeddings returns all stored embeddings for the given model.
	ListEmbeddings(model string) (map[string]Embedding, error)
	Close() error
}

// Embedding is a stored vector derived from a resource's non-secret metadata
// (never its value). Model and Dim are stamped so vectors from different
// embedding spaces are never silently compared; TextHash detects staleness.
type Embedding struct {
	Key      string
	Model    string
	Dim      int
	Vector   []float32
	TextHash string
}
