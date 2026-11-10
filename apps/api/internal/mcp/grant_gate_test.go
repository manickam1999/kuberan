package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"kuberan/internal/services"
	"kuberan/internal/testutil"
)

// fakeGrants is a GrantAuthorizer with canned answers and call counters.
type fakeGrants struct {
	mu         sync.Mutex
	active     map[string]bool
	activeErr  error
	touchErr   error
	isActiveN  int
	touchCalls []string
}

func (f *fakeGrants) Authorize(_, _, _ string) error { return nil }

func (f *fakeGrants) IsActive(userID, clientID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isActiveN++
	if f.activeErr != nil {
		return false, f.activeErr
	}
	return f.active[userID+"|"+clientID], nil
}

func (f *fakeGrants) Touch(userID, clientID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touchCalls = append(f.touchCalls, userID+"|"+clientID)
	return f.touchErr
}

func serveWithGate(gate *grantGate, claims *AccessClaims) *httptest.ResponseRecorder {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := withWWWAuthenticate(requireBearer(next, fakeValidator{claims: claims}, nil, gate), testOAuthConfig)
	req := httptest.NewRequest("POST", "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func inlineGate(g GrantAuthorizer, now func() time.Time) *grantGate {
	gate := newGrantGate(g, grantTTL, now)
	gate.background = func(f func()) { f() }
	return gate
}

func TestRequireBearerRefusesRevokedGrant(t *testing.T) {
	grants := &fakeGrants{active: map[string]bool{"u1|c1": true}}
	now := time.Unix(1_700_000_000, 0)
	gate := inlineGate(grants, func() time.Time { return now })
	claims := &AccessClaims{Subject: "u1", ClientID: "c1", Scopes: []string{"read:accounts"}}

	if rec := serveWithGate(gate, claims); rec.Code != http.StatusOK {
		t.Fatalf("active grant: status %d, want 200", rec.Code)
	}

	// The user disconnects the app. Within the cache TTL reads still pass; once
	// it expires they are refused with the same 401 an invalid token gets.
	grants.active["u1|c1"] = false
	if rec := serveWithGate(gate, claims); rec.Code != http.StatusOK {
		t.Fatalf("cached within the TTL: status %d, want 200", rec.Code)
	}
	now = now.Add(grantTTL + time.Second)
	rec := serveWithGate(gate, claims)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked grant after the TTL: status %d, want 401", rec.Code)
	}
	invalid := httptest.NewRecorder()
	writeUnauthorized(invalid)
	if rec.Body.String() != invalid.Body.String() {
		t.Errorf("body %q differs from the invalid-token body %q", rec.Body.String(), invalid.Body.String())
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("the 401 should carry the WWW-Authenticate challenge so the client reconnects")
	}
}

func TestRequireBearerRefusesLegacyConnection(t *testing.T) {
	// No grant row at all (connected before grants existed) is refused.
	grants := &fakeGrants{active: map[string]bool{}}
	gate := inlineGate(grants, time.Now)
	if rec := serveWithGate(gate, &AccessClaims{Subject: "u1", ClientID: "legacy"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy connection: status %d, want 401", rec.Code)
	}
	// A token with no client_id can't be matched to a grant either.
	if rec := serveWithGate(gate, &AccessClaims{Subject: "u1"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token without client_id: status %d, want 401", rec.Code)
	}
	if len(grants.touchCalls) != 0 {
		t.Errorf("a refused request must not record use, got %v", grants.touchCalls)
	}
}

func TestRequireBearerGrantLookupFailure(t *testing.T) {
	grants := &fakeGrants{activeErr: errors.New("db down")}
	gate := inlineGate(grants, time.Now)
	if rec := serveWithGate(gate, &AccessClaims{Subject: "u1", ClientID: "c1"}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("lookup failure: status %d, want 503", rec.Code)
	}
	// A failure is not cached: the next request asks again.
	grants.activeErr = nil
	grants.active = map[string]bool{"u1|c1": true}
	if rec := serveWithGate(gate, &AccessClaims{Subject: "u1", ClientID: "c1"}); rec.Code != http.StatusOK {
		t.Fatalf("after recovery: status %d, want 200", rec.Code)
	}
}

func TestRequireBearerTouchesAndSurvivesTouchFailure(t *testing.T) {
	grants := &fakeGrants{active: map[string]bool{"u1|c1": true}, touchErr: errors.New("update failed")}
	gate := inlineGate(grants, time.Now)
	claims := &AccessClaims{Subject: "u1", ClientID: "c1"}

	// A touch failure is logged and never fails the request.
	if rec := serveWithGate(gate, claims); rec.Code != http.StatusOK {
		t.Fatalf("touch failure must not fail the request: status %d", rec.Code)
	}
	if len(grants.touchCalls) != 1 || grants.touchCalls[0] != "u1|c1" {
		t.Fatalf("expected one touch for u1|c1, got %v", grants.touchCalls)
	}
	// The cached grant check does not skip the per-request touch (the grant
	// service throttles it).
	_ = serveWithGate(gate, claims)
	if len(grants.touchCalls) != 2 || grants.isActiveN != 1 {
		t.Errorf("touch calls = %d (want 2), grant lookups = %d (want 1)", len(grants.touchCalls), grants.isActiveN)
	}
}

func TestRequireBearerTouchRunsInTheBackground(t *testing.T) {
	grants := &fakeGrants{active: map[string]bool{"u1|c1": true}}
	gate := newGrantGate(grants, grantTTL, time.Now)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	gate.background = func(f func()) {
		go func() {
			close(started)
			<-release
			f()
			close(done)
		}()
	}
	// The request completes while the touch is still blocked.
	if rec := serveWithGate(gate, &AccessClaims{Subject: "u1", ClientID: "c1"}); rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	<-started
	close(release)
	<-done
	if len(grants.touchCalls) != 1 {
		t.Errorf("expected the background touch to run once, got %d", len(grants.touchCalls))
	}
}

// TestGrantGateWithRealService wires the gate to the real grant service: a
// disconnect (Revoke) is refused for reads once the cache expires, and a read
// records last_used_at.
func TestGrantGateWithRealService(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	user := testutil.CreateTestUser(t, db)
	svc := services.NewOAuthGrantService(db)
	if _, err := svc.RecordConsent(services.RecordConsentInput{
		UserID: user.ID, ClientID: "c1", ClientName: "Claude",
		Approved: []string{"read:accounts"}, Reviewed: []string{"read:accounts"},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	gate := inlineGate(svc, func() time.Time { return now })
	claims := &AccessClaims{Subject: user.ID, ClientID: "c1", Scopes: []string{"read:accounts"}}

	if rec := serveWithGate(gate, claims); rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	g, err := svc.Get(user.ID, "c1")
	testutil.AssertNoError(t, err)
	if g.LastUsedAt == nil {
		t.Error("a read-only request should record last_used_at")
	}

	testutil.AssertNoError(t, svc.Revoke(user.ID, "c1"))
	now = now.Add(grantTTL + time.Second)
	if rec := serveWithGate(gate, claims); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked: status %d, want 401", rec.Code)
	}
}
