package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestMemBlobStore_PutGetRoundTrip(t *testing.T) {
	store := NewMemBlobStore()
	ctx := context.Background()
	want := []byte("receipt-bytes")

	if err := store.Put(ctx, "u/tx/a.jpg", bytes.NewReader(want), "image/jpeg", int64(len(want))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, "u/tx/a.jpg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, want)
	}
}

func TestMemBlobStore_GetMissing(t *testing.T) {
	store := NewMemBlobStore()
	if _, err := store.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemBlobStore_Delete(t *testing.T) {
	store := NewMemBlobStore()
	ctx := context.Background()

	if err := store.Put(ctx, "k", strings.NewReader("x"), "text/plain", 1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	// Deleting a missing key is a no-op.
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestMemBlobStore_GetReturnsCopy(t *testing.T) {
	store := NewMemBlobStore()
	ctx := context.Background()
	if err := store.Put(ctx, "k", strings.NewReader("abc"), "text/plain", 3); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	got[0] = 'z' // mutate the returned buffer

	rc2, err := store.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	defer rc2.Close()
	got2, _ := io.ReadAll(rc2)
	if string(got2) != "abc" {
		t.Fatalf("stored bytes were mutated via returned buffer: %q", got2)
	}
}
