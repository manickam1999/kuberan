// Package storage provides a swappable object-storage abstraction for binary
// blobs (receipt attachments). The production implementation targets an
// S3-compatible backend (self-hosted MinIO); an in-memory implementation backs
// unit tests. See plans/017-transaction-receipts.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned by a BlobStore when the requested key does not exist.
var ErrNotFound = errors.New("storage: object not found")

// BlobStore is a minimal object-storage contract. Implementations must be safe
// for concurrent use.
type BlobStore interface {
	// Put stores the bytes read from r under key. size is the exact number of
	// bytes and contentType the MIME type to persist as object metadata.
	Put(ctx context.Context, key string, r io.Reader, contentType string, size int64) error
	// Get returns a reader for the object at key. The caller must Close it.
	// It returns ErrNotFound if the key does not exist.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}
