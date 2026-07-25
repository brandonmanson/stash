package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"stash/internal/resource"
)

func testStore(t *testing.T) Store {
	t.Helper()
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestStoreFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	st, err := OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("store file must be 0600, got %o", perm)
	}
}

func TestReserveFillCycle(t *testing.T) {
	st := testStore(t)
	if _, err := st.Reserve(resource.Resource{Key: "acme.resend.credentials.key", Type: "credential"}); err != nil {
		t.Fatal(err)
	}
	// Idempotent re-reserve.
	if _, err := st.Reserve(resource.Resource{Key: "acme.resend.credentials.key", Type: "credential"}); err != nil {
		t.Fatalf("re-reserve should be a no-op, got %v", err)
	}
	res, err := st.Get("acme.resend.credentials.key")
	if err != nil || !res.Reserved {
		t.Fatalf("expected reserved resource, got %+v, err %v", res, err)
	}
	// Fill clears the flag.
	if _, err := st.Put(resource.Resource{Key: "acme.resend.credentials.key", Type: "credential", Value: []byte("ct")}); err != nil {
		t.Fatal(err)
	}
	res, _ = st.Get("acme.resend.credentials.key")
	if res.Reserved {
		t.Fatal("fill should clear the reserved flag")
	}
	// Reserving a filled key errors.
	if _, err := st.Reserve(resource.Resource{Key: "acme.resend.credentials.key", Type: "credential"}); err == nil {
		t.Fatal("reserving a filled key should error")
	}
}

func TestLazyResolutionDissolvesReservedAncestor(t *testing.T) {
	st := testStore(t)
	if _, err := st.Reserve(resource.Resource{Key: "agency.engagements.newclient", Type: "note"}); err != nil {
		t.Fatal(err)
	}
	dissolved, err := st.Put(resource.Resource{
		Key: "agency.engagements.newclient.vercel.credentials.api_key", Type: "credential", Value: []byte("ct")})
	if err != nil {
		t.Fatal(err)
	}
	if len(dissolved) != 1 || dissolved[0] != "agency.engagements.newclient" {
		t.Fatalf("expected dissolved [agency.engagements.newclient], got %v", dissolved)
	}
	// The reservation row is gone; the deep leaf exists.
	var nfe *NotFoundError
	if _, err := st.Get("agency.engagements.newclient"); !errors.As(err, &nfe) {
		t.Fatalf("dissolved reservation should be deleted, got %v", err)
	}
	if _, err := st.Get("agency.engagements.newclient.vercel.credentials.api_key"); err != nil {
		t.Fatal(err)
	}
	// Reserve-under-reserve also dissolves.
	if _, err := st.Reserve(resource.Resource{Key: "x.pending", Type: "note"}); err != nil {
		t.Fatal(err)
	}
	dissolved, err = st.Reserve(resource.Resource{Key: "x.pending.deeper", Type: "note"})
	if err != nil || len(dissolved) != 1 {
		t.Fatalf("reserve under reservation should dissolve it, got %v, err %v", dissolved, err)
	}
}

func TestDissolutionIsDownwardOnly(t *testing.T) {
	st := testStore(t)
	// A specific reservation blocks a coarse put — specific beats coarse.
	if _, err := st.Reserve(resource.Resource{Key: "a.b.c", Type: "token"}); err != nil {
		t.Fatal(err)
	}
	var collision *CollisionError
	if _, err := st.Put(resource.Resource{Key: "a.b", Type: "note", Value: []byte("x")}); !errors.As(err, &collision) {
		t.Fatalf("coarse put should collide with deeper reservation, got %v", err)
	}
	// A filled ancestor still blocks — only reservations dissolve.
	if _, err := st.Put(resource.Resource{Key: "p.q", Type: "note", Value: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(resource.Resource{Key: "p.q.r", Type: "note", Value: []byte("x")}); !errors.As(err, &collision) {
		t.Fatalf("put under a filled leaf should collide, got %v", err)
	}
}

func TestListReservedFlag(t *testing.T) {
	st := testStore(t)
	st.Reserve(resource.Resource{Key: "x.pending", Type: "token"})
	st.Put(resource.Resource{Key: "x.done", Type: "note", Value: []byte("v")})
	entries, err := st.List("x")
	if err != nil || len(entries) != 2 {
		t.Fatalf("want 2 entries, got %v, err %v", entries, err)
	}
	for _, e := range entries {
		want := e.Key == "x.pending"
		if e.Reserved != want {
			t.Errorf("entry %s: reserved = %v, want %v", e.Key, e.Reserved, want)
		}
	}
}

func TestDescriptionAndEmbeddings(t *testing.T) {
	st := testStore(t)
	if _, err := st.Put(resource.Resource{Key: "subs.billshark", Type: "note", Value: []byte("v"),
		Description: "service that renegotiated my water bill"}); err != nil {
		t.Fatal(err)
	}
	res, err := st.Get("subs.billshark")
	if err != nil || res.Description != "service that renegotiated my water bill" {
		t.Fatalf("description round-trip failed: %+v err %v", res, err)
	}
	// Description is plain-searchable.
	hits, err := st.Search("water bill")
	if err != nil || len(hits) != 1 {
		t.Fatalf("search by description: got %v, err %v", hits, err)
	}
	// Embedding round-trip and delete cascade.
	emb := Embedding{Key: "subs.billshark", Model: "m1", Dim: 3, Vector: []float32{0.1, -0.5, 1}, TextHash: "abc"}
	if err := st.PutEmbedding(emb); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListEmbeddings("m1")
	if err != nil || len(got) != 1 || got["subs.billshark"].Vector[2] != 1 {
		t.Fatalf("embedding round-trip: %v err %v", got, err)
	}
	if err := st.Delete("subs.billshark"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ListEmbeddings("m1"); len(got) != 0 {
		t.Fatal("delete should cascade to embeddings")
	}
}
