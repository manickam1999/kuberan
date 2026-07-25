package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestDisabledBlobStore verifies every operation fails with ErrDisabled so a
// misconfigured instance surfaces a clear error instead of a nil-pointer panic.
func TestDisabledBlobStore(t *testing.T) {
	s := NewDisabledBlobStore()
	ctx := context.Background()

	if err := s.Put(ctx, "k", strings.NewReader("x"), "image/png", 1); !errors.Is(err, ErrDisabled) {
		t.Errorf("Put: expected ErrDisabled, got %v", err)
	}
	if _, err := s.Get(ctx, "k"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Get: expected ErrDisabled, got %v", err)
	}
	if err := s.Delete(ctx, "k"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Delete: expected ErrDisabled, got %v", err)
	}
}
