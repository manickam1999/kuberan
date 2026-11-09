package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestOAuthGrantService_TouchAndHost(t *testing.T) {
	setup := func(t *testing.T) (*oauthGrantService, *models.User) {
		t.Helper()
		db := testutil.SetupTestDB(t)
		t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
		svc := NewOAuthGrantService(db).(*oauthGrantService)
		user := testutil.CreateTestUser(t, db)
		_, err := svc.RecordConsent(RecordConsentInput{
			UserID: user.ID, ClientID: "cid", ClientName: "Claude",
			Approved: []string{"read:accounts"}, Reviewed: []string{"read:accounts"}, ClientHost: "claude.ai",
		})
		testutil.AssertNoError(t, err)
		return svc, user
	}
	lastUsed := func(t *testing.T, svc *oauthGrantService, userID string) *time.Time {
		t.Helper()
		g, err := svc.Get(userID, "cid")
		testutil.AssertNoError(t, err)
		if g.LastUsedAt == nil {
			return nil
		}
		u := g.LastUsedAt.UTC()
		return &u
	}
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("touch_records_a_read_only_use", func(t *testing.T) {
		svc, user := setup(t)
		svc.now = func() time.Time { return base }
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))
		if got := lastUsed(t, svc, user.ID); got == nil || !got.Equal(base) {
			t.Errorf("expected last_used_at %v, got %v", base, got)
		}
	})

	t.Run("touch_is_throttled_in_process", func(t *testing.T) {
		svc, user := setup(t)
		svc.now = func() time.Time { return base }
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))

		// Break the SQL path: a throttled call must not reach the database at all.
		testutil.AssertNoError(t, svc.db.Exec("DROP TABLE oauth_client_grants").Error)
		svc.now = func() time.Time { return base.Add(30 * time.Second) }
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))

		// Past the interval it does hit the (now missing) table, and reports it.
		svc.now = func() time.Time { return base.Add(2 * time.Minute) }
		if err := svc.Touch(user.ID, "cid"); err == nil {
			t.Error("expected the touch after the interval to run its UPDATE")
		}
	})

	t.Run("touch_is_throttled_in_sql_across_replicas", func(t *testing.T) {
		svc, user := setup(t)
		replica := NewOAuthGrantService(svc.db).(*oauthGrantService) // empty in-process throttle

		svc.now = func() time.Time { return base }
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))
		replica.now = func() time.Time { return base.Add(30 * time.Second) }
		testutil.AssertNoError(t, replica.Touch(user.ID, "cid"))
		if got := lastUsed(t, svc, user.ID); got == nil || !got.Equal(base) {
			t.Errorf("the UPDATE guard should keep %v, got %v", base, got)
		}

		replica.now = func() time.Time { return base.Add(2 * time.Minute) }
		replica.touched = map[string]time.Time{}
		testutil.AssertNoError(t, replica.Touch(user.ID, "cid"))
		if got := lastUsed(t, svc, user.ID); got == nil || !got.Equal(base.Add(2*time.Minute)) {
			t.Errorf("expected last_used_at to advance, got %v", got)
		}
	})

	t.Run("touch_failure_is_retried_not_throttled", func(t *testing.T) {
		svc, user := setup(t)
		svc.now = func() time.Time { return base }
		testutil.AssertNoError(t, svc.db.Exec("ALTER TABLE oauth_client_grants RENAME TO grants_away").Error)
		if err := svc.Touch(user.ID, "cid"); err == nil {
			t.Fatal("expected an error with the table missing")
		}
		testutil.AssertNoError(t, svc.db.Exec("ALTER TABLE grants_away RENAME TO oauth_client_grants").Error)
		svc.now = func() time.Time { return base.Add(time.Second) }
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))
		if got := lastUsed(t, svc, user.ID); got == nil {
			t.Error("the retry right after a failure should record the use")
		}
	})

	t.Run("touch_ignores_revoked_and_unknown", func(t *testing.T) {
		svc, user := setup(t)
		svc.now = func() time.Time { return base }
		testutil.AssertNoError(t, svc.Touch(user.ID, "nope"))
		testutil.AssertNoError(t, svc.Revoke(user.ID, "cid"))
		testutil.AssertNoError(t, svc.Touch(user.ID, "cid"))
		if got := lastUsed(t, svc, user.ID); got != nil {
			t.Errorf("a revoked grant must not record use, got %v", got)
		}
	})

	t.Run("is_active", func(t *testing.T) {
		svc, user := setup(t)
		other := testutil.CreateTestUser(t, svc.db)
		for _, tc := range []struct {
			user, client string
			want         bool
		}{{user.ID, "cid", true}, {user.ID, "legacy-only", false}, {other.ID, "cid", false}, {user.ID, "", false}} {
			got, err := svc.IsActive(tc.user, tc.client)
			testutil.AssertNoError(t, err)
			if got != tc.want {
				t.Errorf("IsActive(%q,%q) = %v, want %v", tc.user, tc.client, got, tc.want)
			}
		}
		testutil.AssertNoError(t, svc.Revoke(user.ID, "cid"))
		if got, _ := svc.IsActive(user.ID, "cid"); got {
			t.Error("a revoked grant is not active")
		}
		// Reconnecting clears the revocation.
		_, err := svc.RecordConsent(RecordConsentInput{UserID: user.ID, ClientID: "cid", ClientName: "Claude", Approved: []string{"read:accounts"}})
		testutil.AssertNoError(t, err)
		if got, _ := svc.IsActive(user.ID, "cid"); !got {
			t.Error("a reconnected grant is active again")
		}
	})

	t.Run("client_host_recorded_and_kept", func(t *testing.T) {
		svc, user := setup(t)
		g, err := svc.Get(user.ID, "cid")
		testutil.AssertNoError(t, err)
		if g.ClientHost != "claude.ai" {
			t.Errorf("RecordConsent should store the host, got %q", g.ClientHost)
		}
		// A re-consent without a host (native app) keeps the recorded one.
		_, err = svc.RecordConsent(RecordConsentInput{UserID: user.ID, ClientID: "cid", ClientName: "Claude", Approved: []string{"read:accounts"}})
		testutil.AssertNoError(t, err)
		g, _ = svc.Get(user.ID, "cid")
		if g.ClientHost != "claude.ai" {
			t.Errorf("an empty host must not erase the recorded one, got %q", g.ClientHost)
		}
		// SetClientHost (the auto-accept path) updates it, and ignores empties/unknowns.
		testutil.AssertNoError(t, svc.SetClientHost(user.ID, "cid", "chatgpt.com"))
		testutil.AssertNoError(t, svc.SetClientHost(user.ID, "cid", ""))
		testutil.AssertNoError(t, svc.SetClientHost(user.ID, "unknown", "x.example"))
		g, _ = svc.Get(user.ID, "cid")
		if g.ClientHost != "chatgpt.com" {
			t.Errorf("got %q", g.ClientHost)
		}
	})
}
