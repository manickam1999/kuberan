package mcp

import (
	"sync"
	"time"

	"kuberan/internal/logger"
)

// grantTTL is how long a client's grant state is cached per user and app. It
// matches activeUserTTL: disconnecting an app stops its reads within this
// long, instead of when its access token expires (up to 15 minutes).
const grantTTL = time.Minute

// maxGrantEntries bounds the cache; expired entries are swept when it fills.
const maxGrantEntries = 10_000

// grantGate is the read-path check that the calling app still holds a
// non-revoked grant, plus the per-request "last used" bookkeeping. Writes keep
// their own uncached check in Authorize.
type grantGate struct {
	grants GrantAuthorizer
	ttl    time.Duration
	now    func() time.Time
	// background runs bookkeeping off the request path; tests run it inline.
	background func(func())

	mu      sync.Mutex
	entries map[string]grantEntry
}

type grantEntry struct {
	active    bool
	checkedAt time.Time
}

func newGrantGate(grants GrantAuthorizer, ttl time.Duration, now func() time.Time) *grantGate {
	return &grantGate{
		grants:     grants,
		ttl:        ttl,
		now:        now,
		background: func(f func()) { go f() },
		entries:    map[string]grantEntry{},
	}
}

// isActive reports whether userID holds a non-revoked grant for clientID. A
// missing grant (a legacy connection made before grants existed) is not
// active. A lookup failure is returned and not cached.
func (g *grantGate) isActive(userID, clientID string) (bool, error) {
	if userID == "" || clientID == "" {
		return false, nil
	}
	key := userID + "|" + clientID
	now := g.now()
	g.mu.Lock()
	e, ok := g.entries[key]
	g.mu.Unlock()
	if ok && now.Sub(e.checkedAt) < g.ttl {
		return e.active, nil
	}

	active, err := g.grants.IsActive(userID, clientID)
	if err != nil {
		return false, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.entries) >= maxGrantEntries {
		for k, old := range g.entries {
			if now.Sub(old.checkedAt) >= g.ttl {
				delete(g.entries, k)
			}
		}
	}
	if len(g.entries) < maxGrantEntries {
		g.entries[key] = grantEntry{active: active, checkedAt: now}
	}
	return active, nil
}

// touch records a use of the app in the background. A failure is logged and
// never fails the request; the grant service throttles the writes.
func (g *grantGate) touch(userID, clientID string) {
	g.background(func() {
		if err := g.grants.Touch(userID, clientID); err != nil {
			logger.Get().Warnw("MCP auth: failed to record app use", "error", err, "client_id", clientID)
		}
	})
}
