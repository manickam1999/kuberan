package services

import (
	"context"
	"errors"
	"testing"

	"kuberan/internal/hydra"
	"kuberan/internal/testutil"
)

// revokeAllHydra is a hydra.AdminClient whose only implemented method is
// RevokeAllSessions; the embedded nil interface panics on anything else.
type revokeAllHydra struct {
	hydra.AdminClient
	subject string
	err     error
}

func (h *revokeAllHydra) RevokeAllSessions(_ context.Context, subject string) error {
	h.subject = subject
	return h.err
}

func TestRevokeUserOAuthAccess(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	grants := NewOAuthGrantService(db)
	user := testutil.CreateTestUser(t, db)
	other := testutil.CreateTestUser(t, db)
	for _, in := range []RecordConsentInput{
		{UserID: user.ID, ClientID: "a", Approved: []string{"read:accounts"}, AutoAccept: true},
		{UserID: user.ID, ClientID: "b", Approved: []string{"write:transactions"}},
		{UserID: other.ID, ClientID: "a", Approved: []string{"read:accounts"}},
	} {
		_, err := grants.RecordConsent(in)
		testutil.AssertNoError(t, err)
	}

	// A Hydra failure changes nothing, so the call can be retried.
	failing := &revokeAllHydra{err: errors.New("hydra down")}
	if err := RevokeUserOAuthAccess(context.Background(), failing, grants, user.ID); err == nil {
		t.Fatal("expected the Hydra failure to surface")
	}
	if list, _ := grants.List(user.ID); len(list) != 2 {
		t.Fatalf("grants must stay active when Hydra fails, got %d", len(list))
	}

	h := &revokeAllHydra{}
	testutil.AssertNoError(t, RevokeUserOAuthAccess(context.Background(), h, grants, user.ID))
	if h.subject != user.ID {
		t.Errorf("Hydra sessions revoked for %q, want %q", h.subject, user.ID)
	}
	if list, _ := grants.List(user.ID); len(list) != 0 {
		t.Errorf("all of the user's grants must be revoked, %d remain", len(list))
	}
	g, err := grants.Get(user.ID, "a")
	testutil.AssertNoError(t, err)
	if g.AutoAccept {
		t.Error("a revoked grant must stop auto-accepting")
	}
	if err := grants.Authorize(user.ID, "b", "write:transactions"); err == nil {
		t.Error("writes must be refused after revocation")
	}
	if list, _ := grants.List(other.ID); len(list) != 1 {
		t.Error("other users' grants must not be touched")
	}

	n, err := grants.RevokeAll(user.ID)
	testutil.AssertNoError(t, err)
	if n != 0 {
		t.Errorf("revoking again should be a no-op, revoked %d", n)
	}
}
