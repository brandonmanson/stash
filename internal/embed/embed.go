// Package embed turns resource metadata into vectors for recall.
//
// Security invariant: the embedding path receives Entry (metadata) only —
// resource values structurally cannot reach it. Descriptions, keys, tags,
// and types are non-secret by construction (see DECISIONS.md).
//
// Inference is compiled in behind the `llama` build tag (statically linked
// llama.cpp); model weights are fetched once to <home>/models on first use.
package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"stash/internal/resource"
)

// Model describes a known GGUF embedding model. Prefixes are model-specific
// conventions (e.g. nomic's search_document:/search_query:) that materially
// affect retrieval quality.
type Model struct {
	Name        string
	URL         string
	File        string
	Dim         int
	DocPrefix   string
	QueryPrefix string
}

// Registry returns the curated model set. Default first.
func Registry() []Model {
	return []Model{
		{
			Name: "bge-small-en-v1.5",
			URL:  "https://huggingface.co/CompendiumLabs/bge-small-en-v1.5-gguf/resolve/main/bge-small-en-v1.5-q8_0.gguf",
			File: "bge-small-en-v1.5-q8_0.gguf",
			Dim:  384,
			// BGE v1.5 recommends an instruction prefix on queries only.
			QueryPrefix: "Represent this sentence for searching relevant passages: ",
		},
		{
			Name:        "nomic-embed-text-v1.5",
			URL:         "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q8_0.gguf",
			File:        "nomic-embed-text-v1.5.Q8_0.gguf",
			Dim:         768,
			DocPrefix:   "search_document: ",
			QueryPrefix: "search_query: ",
		},
	}
}

// Active returns the selected model: $STASH_EMBED_MODEL by name, else the
// registry default.
func Active() (Model, error) {
	models := Registry()
	name := os.Getenv("STASH_EMBED_MODEL")
	if name == "" {
		return models[0], nil
	}
	for _, m := range models {
		if m.Name == name {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("unknown embed model %q (known: %s)", name, strings.Join(modelNames(), ", "))
}

func modelNames() []string {
	models := Registry()
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.Name
	}
	return out
}

// Text composes the embedded text for an entry: the description leads (it is
// written for retrieval), with key words, type, and tags as supporting
// signal. Deterministic — its hash detects staleness.
func Text(e resource.Entry) string {
	parts := []string{}
	if e.Description != "" {
		parts = append(parts, e.Description)
	}
	parts = append(parts, strings.ReplaceAll(e.Key, ".", " "), e.Type)
	if len(e.Tags) > 0 {
		parts = append(parts, strings.Join(e.Tags, " "))
	}
	return strings.Join(parts, " | ")
}

// Hash fingerprints the composed text plus model identity.
func Hash(text, model string) string {
	h := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(h[:8])
}

// Cosine returns the cosine similarity of two same-length vectors.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// EnsureModel downloads the model file into <home>/models if absent and
// returns its path. The download is announced on stderr — it is a one-time,
// visible event, not a hidden dependency.
func EnsureModel(home string, m Model) (string, error) {
	dir := filepath.Join(home, "models")
	path := filepath.Join(dir, m.File)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating models dir: %w", err)
	}
	fmt.Fprintf(os.Stderr, "downloading embedding model %s (one-time, to %s)...\n", m.Name, dir)
	resp, err := http.Get(m.URL)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", m.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %s", m.Name, resp.Status)
	}
	tmp := path + ".partial"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("creating model file: %w", err)
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("downloading %s: %w", m.Name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("finalizing model download: %w", err)
	}
	fmt.Fprintf(os.Stderr, "downloaded %s (%.1f MB)\n", m.File, float64(n)/1e6)
	return path, nil
}

// Embedder produces vectors. The real implementation is compiled in behind
// the `llama` build tag; without it, Open returns a helpful error.
type Embedder interface {
	Embed(text string) ([]float32, error)
	Close()
}

// Open loads the active model and returns a ready Embedder.
func Open(home string) (Embedder, Model, error) {
	m, err := Active()
	if err != nil {
		return nil, Model{}, fmt.Errorf("selecting embed model: %w", err)
	}
	path, err := EnsureModel(home, m)
	if err != nil {
		return nil, Model{}, fmt.Errorf("ensuring embed model: %w", err)
	}
	e, err := newBackend(path)
	if err != nil {
		return nil, Model{}, fmt.Errorf("loading embed model: %w", err)
	}
	return e, m, nil
}
