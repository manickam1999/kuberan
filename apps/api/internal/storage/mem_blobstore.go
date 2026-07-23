package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// MemBlobStore is an in-memory BlobStore for unit tests. It is safe for
// concurrent use.
type MemBlobStore struct {
	mu      sync.RWMutex
	objects map[string]memObject
}

type memObject struct {
	data        []byte
	contentType string
}

// NewMemBlobStore returns an empty in-memory blob store.
func NewMemBlobStore() *MemBlobStore {
	return &MemBlobStore{objects: make(map[string]memObject)}
}

// Put reads all of r into memory and stores it under key.
func (m *MemBlobStore) Put(_ context.Context, key string, r io.Reader, contentType string, _ int64) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memObject{data: data, contentType: contentType}
	return nil
}

// Get returns a reader over a copy of the stored bytes, or ErrNotFound.
func (m *MemBlobStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, ok := m.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	buf := make([]byte, len(obj.data))
	copy(buf, obj.data)
	return io.NopCloser(bytes.NewReader(buf)), nil
}

// Delete removes key. Deleting a missing key is a no-op.
func (m *MemBlobStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}
