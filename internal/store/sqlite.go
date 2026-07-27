package store

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/brandonmanson/stash/internal/resource"
)

const schema = `
CREATE TABLE IF NOT EXISTS resources (
	key        TEXT PRIMARY KEY,
	type       TEXT NOT NULL,
	value      BLOB NOT NULL,
	metadata   TEXT NOT NULL DEFAULT '{}',
	tags       TEXT NOT NULL DEFAULT '[]',
	policy     TEXT NOT NULL DEFAULT '{}',
	reserved   INTEGER NOT NULL DEFAULT 0,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS embeddings (
	key        TEXT PRIMARY KEY,
	model      TEXT NOT NULL,
	dim        INTEGER NOT NULL,
	vector     BLOB NOT NULL,
	text_hash  TEXT NOT NULL,
	updated_at TEXT NOT NULL
);`

type sqliteStore struct {
	conn *sql.DB
}

// OpenSQLite opens (creating if needed) the store at path.
func OpenSQLite(path string) (Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing store schema: %w", err)
	}
	// Metadata (keys, descriptions, tags) is plaintext by design; the store
	// file must not be world-readable. sqlite creates it with umask defaults,
	// so tighten it and its WAL sidecars explicitly.
	for _, sidecar := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(sidecar, 0o600); err != nil && !os.IsNotExist(err) {
			db.Close()
			return nil, fmt.Errorf("restricting store permissions: %w", err)
		}
	}
	// Migrations are applied best-effort for stores created before a column
	// existed; "duplicate column" failures mean the store is already current.
	migrations := []string{
		`ALTER TABLE resources ADD COLUMN reserved INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE resources ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migrating store schema: %w", err)
		}
	}
	return &sqliteStore{conn: db}, nil
}

func (s *sqliteStore) Close() error { return s.conn.Close() }

// checkCollisions enforces leaf-XOR-namespace for key, resolving reserved
// ancestors lazily: an ancestor that is a *reservation* dissolves (is
// deleted) rather than colliding, and its key is returned. A filled ancestor
// still collides, as does an existing child of key.
func (s *sqliteStore) checkCollisions(key string) ([]string, error) {
	// key must not already be a namespace...
	var child string
	err := s.conn.QueryRow(`SELECT key FROM resources WHERE key LIKE ? ESCAPE '\' LIMIT 1`,
		likeEscape(key)+".%").Scan(&child)
	if err == nil {
		return nil, &CollisionError{Key: key, Conflict: child}
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("checking namespace children of %s: %w", key, err)
	}
	// ...and no ancestor of key may be a filled leaf; reserved ancestors
	// dissolve into namespaces.
	var dissolved []string
	for _, anc := range resource.Ancestors(key) {
		var reserved bool
		err := s.conn.QueryRow(`SELECT reserved FROM resources WHERE key = ?`, anc).Scan(&reserved)
		if err == sql.ErrNoRows {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("checking ancestor %s: %w", anc, err)
		}
		if !reserved {
			return nil, &CollisionError{Key: anc, Conflict: key, KeyIsLeaf: true}
		}
		if _, err := s.conn.Exec(`DELETE FROM resources WHERE key = ? AND reserved = 1`, anc); err != nil {
			return nil, fmt.Errorf("dissolving reservation %s: %w", anc, err)
		}
		dissolved = append(dissolved, anc)
	}
	return dissolved, nil
}

func (s *sqliteStore) Put(res resource.Resource) ([]string, error) {
	dissolved, err := s.checkCollisions(res.Key)
	if err != nil {
		return nil, fmt.Errorf("putting %s: %w", res.Key, err)
	}
	meta, tags, policy := marshalJSON(res.Metadata), marshalJSON(res.Tags), marshalJSON(res.Policy)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.conn.Exec(`
		INSERT INTO resources (key, type, value, metadata, tags, policy, reserved, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			type = excluded.type, value = excluded.value, metadata = excluded.metadata,
			tags = excluded.tags, policy = excluded.policy, reserved = 0,
			description = excluded.description, updated_at = excluded.updated_at`,
		res.Key, res.Type, res.Value, meta, tags, policy, res.Description, now, now)
	if err != nil {
		return nil, fmt.Errorf("storing %s: %w", res.Key, err)
	}
	return dissolved, nil
}

func (s *sqliteStore) Reserve(res resource.Resource) ([]string, error) {
	var reserved bool
	err := s.conn.QueryRow(`SELECT reserved FROM resources WHERE key = ?`, res.Key).Scan(&reserved)
	if err == nil {
		if reserved {
			return nil, nil // idempotent: already reserved
		}
		return nil, fmt.Errorf("%q already holds a value; nothing to reserve", res.Key)
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("checking reservation %s: %w", res.Key, err)
	}
	dissolved, err := s.checkCollisions(res.Key)
	if err != nil {
		return nil, fmt.Errorf("reserving %s: %w", res.Key, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.conn.Exec(`
		INSERT INTO resources (key, type, value, metadata, tags, policy, reserved, description, created_at, updated_at)
		VALUES (?, ?, X'', ?, ?, '{}', 1, ?, ?, ?)`,
		res.Key, res.Type, marshalJSON(res.Metadata), marshalJSON(res.Tags), res.Description, now, now)
	if err != nil {
		return nil, fmt.Errorf("reserving %s: %w", res.Key, err)
	}
	return dissolved, nil
}

func (s *sqliteStore) Get(key string) (resource.Resource, error) {
	var res resource.Resource
	var meta, tags, policy, created, updated string
	err := s.conn.QueryRow(`
		SELECT key, type, value, metadata, tags, policy, reserved, description, created_at, updated_at
		FROM resources WHERE key = ?`, key).
		Scan(&res.Key, &res.Type, &res.Value, &meta, &tags, &policy, &res.Reserved, &res.Description, &created, &updated)
	if err == sql.ErrNoRows {
		return res, &NotFoundError{Key: key}
	} else if err != nil {
		return res, fmt.Errorf("loading %s: %w", key, err)
	}
	json.Unmarshal([]byte(meta), &res.Metadata)
	json.Unmarshal([]byte(tags), &res.Tags)
	json.Unmarshal([]byte(policy), &res.Policy)
	res.CreatedAt, _ = time.Parse(time.RFC3339, created)
	res.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return res, nil
}

func (s *sqliteStore) List(prefix string) ([]resource.Entry, error) {
	q := `SELECT key, type, tags, reserved, description, created_at, updated_at FROM resources`
	var args []any
	if prefix != "" {
		q += ` WHERE key = ? OR key LIKE ? ESCAPE '\'`
		args = append(args, prefix, likeEscape(prefix)+".%")
	}
	return s.entries(q+` ORDER BY key`, args...)
}

func (s *sqliteStore) Search(qs string) ([]resource.Entry, error) {
	pat := "%" + likeEscape(strings.ToLower(qs)) + "%"
	return s.entries(`
		SELECT key, type, tags, reserved, description, created_at, updated_at FROM resources
		WHERE lower(key) LIKE ? ESCAPE '\' OR lower(type) LIKE ? ESCAPE '\'
		   OR lower(tags) LIKE ? ESCAPE '\' OR lower(metadata) LIKE ? ESCAPE '\'
		   OR lower(description) LIKE ? ESCAPE '\'
		ORDER BY key`, pat, pat, pat, pat, pat)
}

func (s *sqliteStore) Delete(key string) error {
	res, err := s.conn.Exec(`DELETE FROM resources WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("deleting %s: %w", key, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("deleting %s: %w", key, err)
	}
	if n == 0 {
		return &NotFoundError{Key: key}
	}
	if _, err := s.conn.Exec(`DELETE FROM embeddings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("deleting embedding for %s: %w", key, err)
	}
	return nil
}

func (s *sqliteStore) PutEmbedding(e Embedding) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.conn.Exec(`
		INSERT INTO embeddings (key, model, dim, vector, text_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET model = excluded.model, dim = excluded.dim,
			vector = excluded.vector, text_hash = excluded.text_hash,
			updated_at = excluded.updated_at`,
		e.Key, e.Model, e.Dim, floatsToBlob(e.Vector), e.TextHash, now)
	if err != nil {
		return fmt.Errorf("storing embedding for %s: %w", e.Key, err)
	}
	return nil
}

func (s *sqliteStore) ListEmbeddings(model string) (map[string]Embedding, error) {
	rows, err := s.conn.Query(`SELECT key, model, dim, vector, text_hash FROM embeddings WHERE model = ?`, model)
	if err != nil {
		return nil, fmt.Errorf("listing embeddings: %w", err)
	}
	defer rows.Close()
	out := map[string]Embedding{}
	for rows.Next() {
		var e Embedding
		var blob []byte
		if err := rows.Scan(&e.Key, &e.Model, &e.Dim, &blob, &e.TextHash); err != nil {
			return nil, fmt.Errorf("scanning embedding row: %w", err)
		}
		e.Vector = blobToFloats(blob)
		out[e.Key] = e
	}
	return out, rows.Err()
}

func floatsToBlob(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

func blobToFloats(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func (s *sqliteStore) entries(q string, args ...any) ([]resource.Entry, error) {
	rows, err := s.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing resources: %w", err)
	}
	defer rows.Close()
	var out []resource.Entry
	for rows.Next() {
		var e resource.Entry
		var tags, created, updated string
		if err := rows.Scan(&e.Key, &e.Type, &tags, &e.Reserved, &e.Description, &created, &updated); err != nil {
			return nil, fmt.Errorf("scanning resource row: %w", err)
		}
		json.Unmarshal([]byte(tags), &e.Tags)
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, e)
	}
	return out, rows.Err()
}

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		switch v.(type) {
		case []string:
			return "[]"
		default:
			return "{}"
		}
	}
	return string(b)
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
