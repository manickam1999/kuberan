package mcp

import (
	"errors"
	"sync"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// activeUserTTL is how long a user's active state is cached. Deactivating a
// user cuts off MCP access within this long, even for a client that keeps
// refreshing its access token.
const activeUserTTL = time.Minute

// maxActiveEntries bounds the cache; expired entries are swept when it fills.
const maxActiveEntries = 10_000

// userLookup is the part of services.UserServicer the active-user check needs.
type userLookup interface {
	GetUserByID(id string) (*models.User, error)
}

// activeUsers caches whether each user is active, so every MCP request can
// check it without a database query.
type activeUsers struct {
	users userLookup
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	entries map[string]activeEntry
}

type activeEntry struct {
	active    bool
	checkedAt time.Time
}

func newActiveUsers(users userLookup, ttl time.Duration, now func() time.Time) *activeUsers {
	return &activeUsers{users: users, ttl: ttl, now: now, entries: map[string]activeEntry{}}
}

// isActive reports whether userID exists and is active. A deleted or unknown
// user is inactive; a lookup failure is returned as an error and not cached.
func (a *activeUsers) isActive(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	now := a.now()
	a.mu.Lock()
	e, ok := a.entries[userID]
	a.mu.Unlock()
	if ok && now.Sub(e.checkedAt) < a.ttl {
		return e.active, nil
	}

	active := false
	user, err := a.users.GetUserByID(userID)
	switch {
	case err == nil:
		active = user.IsActive
	case errors.Is(err, apperrors.ErrUserNotFound):
	default:
		return false, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.entries) >= maxActiveEntries {
		for id, old := range a.entries {
			if now.Sub(old.checkedAt) >= a.ttl {
				delete(a.entries, id)
			}
		}
	}
	if len(a.entries) < maxActiveEntries {
		a.entries[userID] = activeEntry{active: active, checkedAt: now}
	}
	return active, nil
}
