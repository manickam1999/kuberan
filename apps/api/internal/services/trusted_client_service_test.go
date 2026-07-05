package services

import (
	"testing"

	"kuberan/internal/testutil"
)

func TestTrustedClientService(t *testing.T) {
	t.Run("untrusted_by_default", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTrustedClientService(db)

		trusted, err := svc.IsTrusted("client-abc")
		testutil.AssertNoError(t, err)
		if trusted {
			t.Fatal("expected unknown client to be untrusted")
		}
	})

	t.Run("trust_then_is_trusted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTrustedClientService(db)

		client, err := svc.Trust("client-abc", "Claude")
		testutil.AssertNoError(t, err)
		if client.ID == "" {
			t.Fatal("expected non-empty client ID")
		}
		if client.ClientID != "client-abc" || client.Name != "Claude" {
			t.Errorf("unexpected client fields: %+v", client)
		}

		trusted, err := svc.IsTrusted("client-abc")
		testutil.AssertNoError(t, err)
		if !trusted {
			t.Fatal("expected client to be trusted after Trust")
		}
	})

	t.Run("trust_is_idempotent", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTrustedClientService(db)

		first, err := svc.Trust("client-abc", "Claude")
		testutil.AssertNoError(t, err)

		second, err := svc.Trust("client-abc", "Claude (again)")
		testutil.AssertNoError(t, err)

		if first.ID != second.ID {
			t.Errorf("expected idempotent Trust to return same record, got %s vs %s", first.ID, second.ID)
		}

		all, err := svc.ListTrusted()
		testutil.AssertNoError(t, err)
		if len(all) != 1 {
			t.Errorf("expected 1 trusted client, got %d", len(all))
		}
	})

	t.Run("empty_client_id_rejected", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTrustedClientService(db)

		_, err := svc.Trust("", "Nameless")
		testutil.AssertAppError(t, err, "INVALID_INPUT")

		_, err = svc.IsTrusted("")
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("list_trusted", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTrustedClientService(db)

		empty, err := svc.ListTrusted()
		testutil.AssertNoError(t, err)
		if len(empty) != 0 {
			t.Errorf("expected empty list, got %d", len(empty))
		}

		_, err = svc.Trust("client-1", "One")
		testutil.AssertNoError(t, err)
		_, err = svc.Trust("client-2", "Two")
		testutil.AssertNoError(t, err)

		all, err := svc.ListTrusted()
		testutil.AssertNoError(t, err)
		if len(all) != 2 {
			t.Errorf("expected 2 trusted clients, got %d", len(all))
		}
	})
}
