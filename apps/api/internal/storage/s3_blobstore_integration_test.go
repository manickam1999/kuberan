//go:build integration

// Package storage integration tests exercise S3BlobStore against a real
// S3-compatible endpoint (the dev MinIO). They are excluded from the default
// build so `go test ./...` stays green without infrastructure.
//
// Run against the dev stack (see docker-compose.yml minio + minio-init) with:
//
//	STORAGE_ENDPOINT=http://localhost:9000 \
//	STORAGE_BUCKET=kuberan-receipts \
//	STORAGE_ACCESS_KEY=<scoped key> \
//	STORAGE_SECRET_KEY=<scoped secret> \
//	go test -tags integration ./internal/storage/ -run TestS3BlobStore -v
//
// The endpoint is reached over path-style addressing (MinIO requirement).
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

// s3StoreFromEnv builds an S3BlobStore from the STORAGE_* env vars, skipping the
// test when the endpoint/credentials are not configured.
func s3StoreFromEnv(t *testing.T) *S3BlobStore {
	t.Helper()
	endpoint := os.Getenv("STORAGE_ENDPOINT")
	bucket := os.Getenv("STORAGE_BUCKET")
	if endpoint == "" || bucket == "" {
		t.Skip("STORAGE_ENDPOINT/STORAGE_BUCKET unset; skipping S3 integration test")
	}
	store, err := NewS3BlobStore(S3Config{
		Endpoint:     endpoint,
		Bucket:       bucket,
		AccessKey:    os.Getenv("STORAGE_ACCESS_KEY"),
		SecretKey:    os.Getenv("STORAGE_SECRET_KEY"),
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3BlobStore: %v", err)
	}
	return store
}

// uniqueKey derives a per-run key so concurrent/repeated runs never collide.
func uniqueKey(t *testing.T) string {
	t.Helper()
	return "integration-test/" + t.Name() + "/" + time.Now().UTC().Format("20060102T150405.000000000")
}

func TestS3BlobStore_PutGetDeleteRoundTrip(t *testing.T) {
	store := s3StoreFromEnv(t)
	ctx := context.Background()
	key := uniqueKey(t)
	want := []byte("integration-receipt-bytes")

	if err := store.Put(ctx, key, bytes.NewReader(want), "image/jpeg", int64(len(want))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Always clean up, even if a later assertion fails.
	t.Cleanup(func() {
		if err := store.Delete(context.Background(), key); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, want)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestS3BlobStore_GetMissingReturnsNotFound(t *testing.T) {
	store := s3StoreFromEnv(t)
	if _, err := store.Get(context.Background(), uniqueKey(t)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing key, got %v", err)
	}
}

func TestS3BlobStore_DeleteMissingIsNoop(t *testing.T) {
	store := s3StoreFromEnv(t)
	if err := store.Delete(context.Background(), uniqueKey(t)); err != nil {
		t.Fatalf("Delete of missing key should be a no-op, got %v", err)
	}
}
