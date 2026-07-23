package storage

import (
	"context"
	"errors"
	"io"
)

// ErrDisabled is returned by every DisabledBlobStore operation. It signals that
// object storage is not configured, so receipt attachments are unavailable.
var ErrDisabled = errors.New("storage: object storage is not configured")

// DisabledBlobStore is a BlobStore that fails every operation with ErrDisabled.
// It lets the API boot when no storage backend is configured (e.g. a dev
// instance without MinIO) instead of coupling the entire server's availability
// to the receipts feature: attachment requests fail cleanly while everything
// else keeps working.
type DisabledBlobStore struct{}

// NewDisabledBlobStore returns a BlobStore whose operations all fail with
// ErrDisabled.
func NewDisabledBlobStore() *DisabledBlobStore { return &DisabledBlobStore{} }

// Put always returns ErrDisabled.
func (DisabledBlobStore) Put(context.Context, string, io.Reader, string, int64) error {
	return ErrDisabled
}

// Get always returns ErrDisabled.
func (DisabledBlobStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrDisabled
}

// Delete always returns ErrDisabled.
func (DisabledBlobStore) Delete(context.Context, string) error { return ErrDisabled }
