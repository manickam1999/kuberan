package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestCreateUser(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		user, err := svc.CreateUser("alice@example.com", "password123", "Alice", "Smith")
		testutil.AssertNoError(t, err)

		if user.ID == "" {
			t.Fatal("expected non-empty user ID")
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected email alice@example.com, got %s", user.Email)
		}
		if user.FirstName != "Alice" {
			t.Errorf("expected first name Alice, got %s", user.FirstName)
		}
		if !user.IsActive {
			t.Error("expected user to be active")
		}
	})

	t.Run("duplicate_email", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("dup@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		_, err = svc.CreateUser("dup@example.com", "password456", "", "")
		testutil.AssertAppError(t, err, "DUPLICATE_EMAIL")
	})

	t.Run("empty_email", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("", "password123", "", "")
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("empty_password", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("test@example.com", "", "", "")
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("email_normalized_to_lowercase", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		user, err := svc.CreateUser("Alice@EXAMPLE.COM", "password123", "", "")
		testutil.AssertNoError(t, err)

		if user.Email != "alice@example.com" {
			t.Errorf("expected lowercased email, got %s", user.Email)
		}
	})
}

func TestGetUserByEmail(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		created := testutil.CreateTestUserWithEmail(t, db, "found@example.com")
		user, err := svc.GetUserByEmail("found@example.com")
		testutil.AssertNoError(t, err)

		if user.ID != created.ID {
			t.Errorf("expected user ID %s, got %s", created.ID, user.ID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.GetUserByEmail("nonexistent@example.com")
		testutil.AssertAppError(t, err, "USER_NOT_FOUND")
	})

	t.Run("inactive_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		user := testutil.CreateTestUserWithEmail(t, db, "inactive@example.com")
		db.Model(user).Update("is_active", false)

		_, err := svc.GetUserByEmail("inactive@example.com")
		testutil.AssertAppError(t, err, "USER_NOT_FOUND")
	})
}

func TestGetUserByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		created := testutil.CreateTestUser(t, db)
		user, err := svc.GetUserByID(created.ID)
		testutil.AssertNoError(t, err)

		if user.Email != created.Email {
			t.Errorf("expected email %s, got %s", created.Email, user.Email)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.GetUserByID("99999")
		testutil.AssertAppError(t, err, "USER_NOT_FOUND")
	})
}

func TestVerifyPassword(t *testing.T) {
	t.Run("correct", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		// Fixture uses "password123" with bcrypt.MinCost
		user := testutil.CreateTestUser(t, db)
		if !svc.VerifyPassword(user, "password123") {
			t.Error("expected password verification to succeed")
		}
	})

	t.Run("incorrect", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		user := testutil.CreateTestUser(t, db)
		if svc.VerifyPassword(user, "wrongpassword") {
			t.Error("expected password verification to fail")
		}
	})
}

func TestAttemptLogin(t *testing.T) {
	t.Run("success_resets_attempts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		// Create user via service so password is hashed with DefaultCost
		_, err := svc.CreateUser("login@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		// Simulate previous failed attempts
		db.Exec("UPDATE users SET failed_login_attempts = 3 WHERE email = ?", "login@example.com")

		user, err := svc.AttemptLogin("login@example.com", "password123")
		testutil.AssertNoError(t, err)

		if user.FailedLoginAttempts != 0 {
			t.Errorf("expected 0 failed attempts after success, got %d", user.FailedLoginAttempts)
		}
		if user.LastLoginAt == nil {
			t.Error("expected LastLoginAt to be set after successful login")
		}
	})

	t.Run("wrong_password_increments_attempts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("fail@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		_, err = svc.AttemptLogin("fail@example.com", "wrongpassword")
		testutil.AssertAppError(t, err, "INVALID_CREDENTIALS")

		// Verify the failed attempts were incremented in DB
		user, _ := svc.GetUserByEmail("fail@example.com")
		if user.FailedLoginAttempts != 1 {
			t.Errorf("expected 1 failed attempt, got %d", user.FailedLoginAttempts)
		}
	})

	t.Run("lockout_after_5_failures", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("lockout@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		// Fail 5 times
		for i := 0; i < 5; i++ {
			_, err = svc.AttemptLogin("lockout@example.com", "wrong")
			testutil.AssertAppError(t, err, "INVALID_CREDENTIALS")
		}

		// Verify account is now locked
		user, _ := svc.GetUserByEmail("lockout@example.com")
		if user.LockedUntil == nil {
			t.Fatal("expected LockedUntil to be set after 5 failures")
		}
		if !user.LockedUntil.After(time.Now()) {
			t.Error("expected LockedUntil to be in the future")
		}
	})

	t.Run("locked_account_returns_error", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("locked@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		// Manually lock the account
		lockUntil := time.Now().Add(15 * time.Minute)
		db.Exec("UPDATE users SET locked_until = ?, failed_login_attempts = 5 WHERE email = ?", lockUntil, "locked@example.com")

		_, err = svc.AttemptLogin("locked@example.com", "password123")
		testutil.AssertAppError(t, err, "ACCOUNT_LOCKED")
	})

	t.Run("nonexistent_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.AttemptLogin("nobody@example.com", "password123")
		testutil.AssertAppError(t, err, "INVALID_CREDENTIALS")
	})

	t.Run("deactivated_account_cannot_authenticate", func(t *testing.T) {
		// This guards the security-critical property that plan 015 Phase 2
		// relies on: the MCP Resource Server no longer checks IsActive per
		// request (it validates Hydra JWTs offline), so a deactivated user must
		// be blocked at login time instead. That enforcement is transitive:
		// GetUserByEmail filters on is_active, so AttemptLogin never sees a
		// deactivated user and returns INVALID_CREDENTIALS (no account
		// enumeration). If GetUserByEmail's is_active filter is ever removed,
		// this test fails and flags that the login-time mitigation regressed.
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)

		_, err := svc.CreateUser("inactive@example.com", "password123", "", "")
		testutil.AssertNoError(t, err)

		// Deactivate the account (e.g. an admin disabled the user).
		if err := db.Model(&models.User{}).
			Where("email = ?", "inactive@example.com").
			Update("is_active", false).Error; err != nil {
			t.Fatalf("failed to deactivate account: %v", err)
		}

		// Correct credentials, but a deactivated account must not authenticate.
		_, err = svc.AttemptLogin("inactive@example.com", "password123")
		testutil.AssertAppError(t, err, "INVALID_CREDENTIALS")
	})
}

func TestStoreAndGetRefreshTokenHash(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewUserService(db)

	user := testutil.CreateTestUser(t, db)

	hash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	err := svc.StoreRefreshTokenHash(user.ID, hash)
	testutil.AssertNoError(t, err)

	got, err := svc.GetRefreshTokenHash(user.ID)
	testutil.AssertNoError(t, err)

	if got != hash {
		t.Errorf("expected hash %s, got %s", hash, got)
	}
}

func TestCreateUser_password_is_hashed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewUserService(db)

	user, err := svc.CreateUser("hash@example.com", "mypassword", "", "")
	testutil.AssertNoError(t, err)

	// Password should be bcrypt hash, not plaintext
	if user.Password == "mypassword" {
		t.Error("password should be hashed, not stored as plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("mypassword")); err != nil {
		t.Error("password hash should be valid bcrypt")
	}
}

func TestUpdateProfileSettings(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	strPtr := func(s string) *string { return &s }
	getUser := func(t *testing.T, svc UserServicer, userID string) *models.User {
		t.Helper()
		u, err := svc.GetUserByID(userID)
		testutil.AssertNoError(t, err)
		return u
	}
	setDefault := func(id *string) ProfileSettingsUpdate {
		return ProfileSettingsUpdate{SetDefaultAccount: true, DefaultAccountID: id}
	}

	t.Run("sets_and_clears_default_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, setDefault(&account.ID)))
		if got := getUser(t, svc, user.ID).DefaultAccountID; got == nil || *got != account.ID {
			t.Fatalf("expected default account %s, got %v", account.ID, got)
		}

		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, setDefault(nil)))
		if got := getUser(t, svc, user.ID).DefaultAccountID; got != nil {
			t.Fatalf("expected default account cleared, got %v", *got)
		}
	})

	t.Run("hide_balances_only_keeps_default_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, setDefault(&account.ID)))

		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{HideBalances: boolPtr(true)}))

		got := getUser(t, svc, user.ID)
		if !got.HideBalances {
			t.Error("expected hide_balances true")
		}
		if got.DefaultAccountID == nil || *got.DefaultAccountID != account.ID {
			t.Errorf("expected default account %s kept, got %v", account.ID, got.DefaultAccountID)
		}
	})

	t.Run("updates_both_settings_together", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.AssertNoError(t, svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			HideBalances: boolPtr(true), SetDefaultAccount: true, DefaultAccountID: &account.ID,
		}))

		got := getUser(t, svc, user.ID)
		if !got.HideBalances || got.DefaultAccountID == nil || *got.DefaultAccountID != account.ID {
			t.Errorf("expected both settings applied, got hide=%v default=%v", got.HideBalances, got.DefaultAccountID)
		}
	})

	t.Run("rejected_default_account_applies_nothing", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		err := svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{
			HideBalances: boolPtr(true), SetDefaultAccount: true, DefaultAccountID: &account.ID,
		})
		testutil.AssertAppError(t, err, "INVALID_INPUT")

		got := getUser(t, svc, user.ID)
		if got.HideBalances || got.DefaultAccountID != nil {
			t.Errorf("expected no change, got hide=%v default=%v", got.HideBalances, got.DefaultAccountID)
		}
	})

	t.Run("requires_a_setting", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewUserService(db)
		user := testutil.CreateTestUser(t, db)

		err := svc.UpdateProfileSettings(user.ID, ProfileSettingsUpdate{})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	rejections := []struct {
		name    string
		account func(t *testing.T, db *gorm.DB, userID string) string
		code    string
	}{
		{"rejects_unknown_account", func(_ *testing.T, _ *gorm.DB, _ string) string {
			return "01a15c66-0000-7000-8000-000000000000"
		}, "ACCOUNT_NOT_FOUND"},
		{"rejects_another_users_account", func(t *testing.T, db *gorm.DB, _ string) string {
			other := testutil.CreateTestUser(t, db)
			return testutil.CreateTestCashAccount(t, db, other.ID).ID
		}, "ACCOUNT_NOT_FOUND"},
		{"rejects_investment_account", func(t *testing.T, db *gorm.DB, userID string) string {
			return testutil.CreateTestInvestmentAccount(t, db, userID).ID
		}, "INVALID_INPUT"},
		{"rejects_inactive_account", func(t *testing.T, db *gorm.DB, userID string) string {
			account := testutil.CreateTestCashAccount(t, db, userID)
			if err := db.Model(account).Update("is_active", false).Error; err != nil {
				t.Fatalf("deactivate account: %v", err)
			}
			return account.ID
		}, "INVALID_INPUT"},
	}
	for _, tc := range rejections {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			svc := NewUserService(db)
			user := testutil.CreateTestUser(t, db)

			err := svc.UpdateProfileSettings(user.ID, setDefault(strPtr(tc.account(t, db, user.ID))))
			testutil.AssertAppError(t, err, tc.code)
			if got := getUser(t, svc, user.ID).DefaultAccountID; got != nil {
				t.Fatalf("expected no default account, got %v", *got)
			}
		})
	}
}

func TestAttemptLogin_keeps_concurrent_profile_changes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewUserService(db)
	user, err := svc.CreateUser("race@example.com", "password123", "Race", "User")
	testutil.AssertNoError(t, err)

	// Simulate a PATCH /profile landing after AttemptLogin has read the user
	// but before it writes the login state back.
	changed := false
	err = db.Callback().Query().After("gorm:query").Register("test:concurrent_profile_change", func(tx *gorm.DB) {
		if changed || tx.Statement.Table != "users" {
			return
		}
		changed = true
		tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE users SET hide_balances = ? WHERE id = ?", true, user.ID)
	})
	testutil.AssertNoError(t, err)

	for _, password := range []string{"wrongpassword", "password123"} {
		changed = false
		testutil.AssertNoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).Update("hide_balances", false).Error)
		_, _ = svc.AttemptLogin("race@example.com", password)

		var got models.User
		testutil.AssertNoError(t, db.First(&got, "id = ?", user.ID).Error)
		if !changed {
			t.Fatalf("password %q: concurrent change was never injected", password)
		}
		if !got.HideBalances {
			t.Errorf("password %q: login overwrote a concurrent hide_balances change", password)
		}
	}
}
