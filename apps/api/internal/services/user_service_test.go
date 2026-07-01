package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"

	"golang.org/x/crypto/bcrypt"
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
