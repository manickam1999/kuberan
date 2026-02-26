package services

import (
	"testing"
	"time"

	"kuberan/internal/testutil"
)

func TestGenerateLinkCode(t *testing.T) {
	t.Run("creates_new_link_for_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)

		if link.LinkCode == "" {
			t.Error("expected non-empty link code")
		}
		if link.LinkCodeExpiresAt == nil {
			t.Fatal("expected non-nil expiration time")
		}
		if time.Until(*link.LinkCodeExpiresAt) < 14*time.Minute {
			t.Error("expected expiration at least 14 minutes from now")
		}
		if link.IsActive {
			t.Error("expected IsActive to be false for new link")
		}
		if link.UserID != user.ID {
			t.Errorf("expected UserID %s, got %s", user.ID, link.UserID)
		}
	})

	t.Run("updates_existing_link_code", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link1, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)
		firstCode := link1.LinkCode
		firstID := link1.ID

		link2, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)

		if link2.ID != firstID {
			t.Errorf("expected same link ID %s, got %s", firstID, link2.ID)
		}
		if link2.LinkCode == firstCode {
			t.Error("expected new code to be different from first code")
		}
		if link2.LinkCode == "" {
			t.Error("expected non-empty link code")
		}
	})

	t.Run("preserves_existing_telegram_data", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		// Create fully linked record
		link := testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)
		originalTelegramUserID := link.TelegramUserID
		originalUsername := link.TelegramUsername

		// Generate new code
		updated, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)

		if updated.TelegramUserID != originalTelegramUserID {
			t.Errorf("expected TelegramUserID preserved as %d, got %d", originalTelegramUserID, updated.TelegramUserID)
		}
		if updated.TelegramUsername != originalUsername {
			t.Errorf("expected TelegramUsername preserved as %s, got %s", originalUsername, updated.TelegramUsername)
		}
		if updated.LinkCode == "" {
			t.Error("expected new link code to be set")
		}
	})
}

func TestCompleteLink(t *testing.T) {
	t.Run("valid_code", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)
		code := link.LinkCode

		err = svc.CompleteLink(code, 987654321, "testuser", "Test", "JPY")
		testutil.AssertNoError(t, err)

		// Verify link was updated
		updated, err := svc.GetLinkByUserID(user.ID)
		testutil.AssertNoError(t, err)

		if updated.TelegramUserID != 987654321 {
			t.Errorf("expected TelegramUserID 987654321, got %d", updated.TelegramUserID)
		}
		if updated.TelegramUsername != "testuser" {
			t.Errorf("expected username testuser, got %s", updated.TelegramUsername)
		}
		if updated.TelegramFirstName != "Test" {
			t.Errorf("expected first name Test, got %s", updated.TelegramFirstName)
		}
		if !updated.IsActive {
			t.Error("expected IsActive to be true")
		}
		if updated.LinkCode != "" {
			t.Error("expected LinkCode to be cleared")
		}
		if updated.LinkCodeExpiresAt != nil {
			t.Error("expected LinkCodeExpiresAt to be cleared")
		}
		if updated.DefaultCurrency != "JPY" {
			t.Errorf("expected DefaultCurrency JPY, got %s", updated.DefaultCurrency)
		}
	})

	t.Run("invalid_code", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)

		err := svc.CompleteLink("invalidcode", 123456789, "user", "User", "USD")
		testutil.AssertAppError(t, err, "INVALID_LINK_CODE")
	})

	t.Run("expired_code", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)
		code := link.LinkCode

		// Manually set expiration to past
		pastTime := time.Now().Add(-1 * time.Hour)
		link.LinkCodeExpiresAt = &pastTime
		db.Save(link)

		err = svc.CompleteLink(code, 123456789, "user", "User", "USD")
		testutil.AssertAppError(t, err, "LINK_CODE_EXPIRED")
	})

	t.Run("telegram_already_linked", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)

		// Link first user
		link1, err := svc.GenerateLinkCode(user1.ID)
		testutil.AssertNoError(t, err)
		code1 := link1.LinkCode
		err = svc.CompleteLink(code1, 111222333, "user1", "User1", "USD")
		testutil.AssertNoError(t, err)

		// Try to link second user with same Telegram ID
		link2, err := svc.GenerateLinkCode(user2.ID)
		testutil.AssertNoError(t, err)
		code2 := link2.LinkCode

		err = svc.CompleteLink(code2, 111222333, "user2", "User2", "USD")
		testutil.AssertAppError(t, err, "TELEGRAM_ALREADY_LINKED")
	})

	t.Run("sets_default_currency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link, err := svc.GenerateLinkCode(user.ID)
		testutil.AssertNoError(t, err)
		code := link.LinkCode

		err = svc.CompleteLink(code, 123456789, "user", "User", "EUR")
		testutil.AssertNoError(t, err)

		updated, err := svc.GetLinkByUserID(user.ID)
		testutil.AssertNoError(t, err)

		if updated.DefaultCurrency != "EUR" {
			t.Errorf("expected DefaultCurrency EUR, got %s", updated.DefaultCurrency)
		}
	})
}

func TestGetLinkByUserID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		created := testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)

		link, err := svc.GetLinkByUserID(user.ID)
		testutil.AssertNoError(t, err)

		if link.ID != created.ID {
			t.Errorf("expected ID %s, got %s", created.ID, link.ID)
		}
		if link.UserID != user.ID {
			t.Errorf("expected UserID %s, got %s", user.ID, link.UserID)
		}
		if link.TelegramUserID != 123456789 {
			t.Errorf("expected TelegramUserID 123456789, got %d", link.TelegramUserID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetLinkByUserID(user.ID)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})
}

func TestGetLinkByTelegramID(t *testing.T) {
	t.Run("found_active", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		created := testutil.CreateTestTelegramLink(t, db, user.ID, 555666777)

		link, err := svc.GetLinkByTelegramID(555666777)
		testutil.AssertNoError(t, err)

		if link.ID != created.ID {
			t.Errorf("expected ID %s, got %s", created.ID, link.ID)
		}
		if link.TelegramUserID != 555666777 {
			t.Errorf("expected TelegramUserID 555666777, got %d", link.TelegramUserID)
		}
	})

	t.Run("not_found_inactive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link := testutil.CreateTestTelegramLink(t, db, user.ID, 888999000)
		link.IsActive = false
		db.Save(link)

		_, err := svc.GetLinkByTelegramID(888999000)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)

		_, err := svc.GetLinkByTelegramID(999999999)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})
}

func TestUnlinkAccount(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)

		err := svc.UnlinkAccount(user.ID)
		testutil.AssertNoError(t, err)

		// Verify link was soft deleted
		_, err = svc.GetLinkByUserID(user.ID)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		err := svc.UnlinkAccount(user.ID)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})
}

func TestRecordActivity(t *testing.T) {
	t.Run("updates_timestamp_and_count", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link := testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)
		if link.MessageCount != 0 {
			t.Errorf("expected initial MessageCount 0, got %d", link.MessageCount)
		}

		err := svc.RecordActivity(123456789)
		testutil.AssertNoError(t, err)

		updated, err := svc.GetLinkByTelegramID(123456789)
		testutil.AssertNoError(t, err)

		if updated.LastMessageAt == nil {
			t.Fatal("expected LastMessageAt to be set")
		}
		if updated.MessageCount != 1 {
			t.Errorf("expected MessageCount 1, got %d", updated.MessageCount)
		}

		// Record again
		err = svc.RecordActivity(123456789)
		testutil.AssertNoError(t, err)

		updated, err = svc.GetLinkByTelegramID(123456789)
		testutil.AssertNoError(t, err)

		if updated.MessageCount != 2 {
			t.Errorf("expected MessageCount 2, got %d", updated.MessageCount)
		}
	})
}

func TestIsLinked(t *testing.T) {
	t.Run("true_when_active", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)

		linked, err := svc.IsLinked(user.ID)
		testutil.AssertNoError(t, err)

		if !linked {
			t.Error("expected IsLinked to return true")
		}
	})

	t.Run("false_when_inactive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link := testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)
		link.IsActive = false
		db.Save(link)

		linked, err := svc.IsLinked(user.ID)
		testutil.AssertNoError(t, err)

		if linked {
			t.Error("expected IsLinked to return false for inactive link")
		}
	})

	t.Run("false_when_no_link", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		linked, err := svc.IsLinked(user.ID)
		testutil.AssertNoError(t, err)

		if linked {
			t.Error("expected IsLinked to return false when no link exists")
		}
	})
}

func TestGetUserWithAuthToken(t *testing.T) {
	t.Run("returns_user_auth", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)
		user := testutil.CreateTestUser(t, db)

		link := testutil.CreateTestTelegramLink(t, db, user.ID, 123456789)

		auth, err := svc.GetUserWithAuthToken(123456789)
		testutil.AssertNoError(t, err)

		if auth.UserID != user.ID {
			t.Errorf("expected UserID %s, got %s", user.ID, auth.UserID)
		}
		if auth.Email != user.Email {
			t.Errorf("expected Email %s, got %s", user.Email, auth.Email)
		}
		if auth.AuthToken == "" {
			t.Error("expected non-empty AuthToken")
		}
		if auth.DefaultCurrency != link.DefaultCurrency {
			t.Errorf("expected DefaultCurrency %s, got %s", link.DefaultCurrency, auth.DefaultCurrency)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewTelegramService(db)

		_, err := svc.GetUserWithAuthToken(999999999)
		testutil.AssertAppError(t, err, "NOT_FOUND")
	})
}
