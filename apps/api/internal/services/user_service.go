package services

import (
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute

	// refreshRotationGrace is how long a rotated-out refresh token stays
	// usable for its session.
	refreshRotationGrace = 2 * time.Minute
	// maxRefreshSessionsPerUser caps concurrently signed-in devices per user.
	maxRefreshSessionsPerUser = 20
)

// userService handles user-related business logic.
type userService struct {
	db *gorm.DB
}

// NewUserService creates a new UserServicer.
func NewUserService(db *gorm.DB) UserServicer {
	return &userService{db: db}
}

// MaxPasswordBytes is the longest password accepted. bcrypt only uses the first
// 72 bytes (and x/crypto rejects longer input), so longer passwords are refused
// up front with a clear error rather than silently truncated or failing late.
const MaxPasswordBytes = 72

// ErrLoginLocked is returned by AttemptLogin when the account is locked. It is
// indistinguishable from ErrInvalidCredentials on the wire (same code, message
// and status) so a lockout does not reveal that the email exists; callers
// compare against it by identity only to audit the refusal distinctly.
var ErrLoginLocked = apperrors.WithMessage(apperrors.ErrInvalidCredentials, apperrors.ErrInvalidCredentials.Message)

// ErrLoginLockoutTriggered is returned by AttemptLogin for the failed attempt
// that locked the account. Like ErrLoginLocked it looks like
// ErrInvalidCredentials to clients.
var ErrLoginLockoutTriggered = apperrors.WithMessage(apperrors.ErrInvalidCredentials, apperrors.ErrInvalidCredentials.Message)

// NormalizeEmail trims and lowercases an email address so lookups and the
// duplicate check agree with how emails are stored.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidatePassword enforces the password length limits shared by every path
// that sets a password.
func ValidatePassword(password string) error {
	if password == "" {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "password is required")
	}
	if len(password) > MaxPasswordBytes {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "password must be at most 72 bytes")
	}
	return nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// dummyPasswordHash returns a bcrypt hash (at the default cost) of a random
// value, used to spend comparable time on logins for unknown emails.
func dummyPasswordHash() []byte {
	dummyHashOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("kuberan-dummy-password"), bcrypt.DefaultCost)
		if err == nil {
			dummyHash = h
		}
	})
	return dummyHash
}

// CreateUser registers a new user
func (s *userService) CreateUser(email, password, firstName, lastName string) (*models.User, error) {
	email = NormalizeEmail(email)
	// Validate input
	if email == "" || password == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "email and password are required")
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	// Check if user with email exists. Unscoped: the unique index on email
	// also covers soft-deleted rows.
	var count int64
	if err := s.db.Unscoped().Model(&models.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if count > 0 {
		return nil, apperrors.ErrDuplicateEmail
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Create user
	user := &models.User{
		Email:     email,
		Password:  string(hashedPassword),
		FirstName: firstName,
		LastName:  lastName,
		IsActive:  true,
	}

	if err := s.db.Create(user).Error; err != nil {
		// A concurrent registration can still win the unique index.
		if isUniqueViolation(err) {
			return nil, apperrors.ErrDuplicateEmail
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return user, nil
}

// isUniqueViolation reports whether err is a unique-constraint violation on
// Postgres or SQLite.
func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "sqlstate 23505")
}

// GetUserByEmail retrieves a user by email
func (s *userService) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("email = ? AND is_active = ?", NormalizeEmail(email), true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by ID
func (s *userService) GetUserByID(id string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &user, nil
}

// VerifyPassword checks if the provided password matches the stored hash
func (s *userService) VerifyPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	return err == nil
}

// AttemptLogin authenticates a user by email and password with lockout protection.
// Returns the user on success, or an appropriate AppError on failure.
//
// Every failure returns a 401 INVALID_CREDENTIALS-shaped error, including a
// locked account (ErrLoginLocked) and the attempt that triggers the lockout
// (ErrLoginLockoutTriggered), so responses never reveal whether an email is
// registered. Unknown emails still pay for a bcrypt comparison to keep timing
// similar.
func (s *userService) AttemptLogin(email, password string) (*models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte(password))
		if errors.Is(err, apperrors.ErrUserNotFound) {
			return nil, apperrors.ErrInvalidCredentials
		}
		return nil, err
	}

	passwordOK := len(password) <= MaxPasswordBytes && s.VerifyPassword(user, password)

	now := time.Now()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return nil, ErrLoginLocked
	}

	if !passwordOK {
		triggered, err := s.recordFailedLogin(user.ID, now)
		if err != nil {
			return nil, err
		}
		if triggered {
			return nil, ErrLoginLockoutTriggered
		}
		return nil, apperrors.ErrInvalidCredentials
	}

	// Successful login: reset failed attempts, unless a concurrent failure
	// locked the account after it was read above.
	res := s.db.Model(&models.User{}).
		Where("id = ? AND (locked_until IS NULL OR locked_until <= ?)", user.ID, now).
		Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
			"last_login_at":         now,
		})
	if res.Error != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrLoginLocked
	}
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &now

	return user, nil
}

// recordFailedLogin counts a failed attempt and locks the account once it
// reaches maxFailedAttempts. Both steps are single conditional UPDATEs, so
// concurrent failures are all counted (no read-modify-write) and exactly one
// of them reports that it triggered the lockout. An expired lock restarts the
// count.
func (s *userService) recordFailedLogin(userID string, now time.Time) (lockoutTriggered bool, err error) {
	expired := "locked_until IS NOT NULL AND locked_until <= ?"
	res := s.db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"failed_login_attempts": gorm.Expr("CASE WHEN "+expired+" THEN 1 ELSE failed_login_attempts + 1 END", now),
		"locked_until":          gorm.Expr("CASE WHEN "+expired+" THEN NULL ELSE locked_until END", now),
	})
	if res.Error != nil {
		return false, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}

	res = s.db.Model(&models.User{}).
		Where("id = ? AND failed_login_attempts >= ? AND (locked_until IS NULL OR locked_until <= ?)",
			userID, maxFailedAttempts, now).
		Update("locked_until", now.Add(lockoutDuration))
	if res.Error != nil {
		return false, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	return res.RowsAffected == 1, nil
}

// CreateRefreshSession starts a new per-device refresh session holding the
// hash of its first refresh token. Expired sessions for the user are pruned,
// and the oldest are dropped beyond maxRefreshSessionsPerUser, so the table
// stays bounded even if clients never log out.
func (s *userService) CreateRefreshSession(userID, tokenHash, userAgent, ipAddress string, expiresAt time.Time) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("user_id = ? AND expires_at <= ?", userID, time.Now()).
			Delete(&models.RefreshSession{}).Error; err != nil {
			return err
		}

		session := models.RefreshSession{
			UserID:    userID,
			TokenHash: tokenHash,
			ExpiresAt: expiresAt,
			UserAgent: truncateRunes(userAgent, 255),
			IPAddress: truncateRunes(ipAddress, 45),
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}

		var stale []string
		if err := tx.Model(&models.RefreshSession{}).Where("user_id = ?", userID).
			Order("created_at DESC").Offset(maxRefreshSessionsPerUser).Pluck("id", &stale).Error; err != nil {
			return err
		}
		if len(stale) > 0 {
			return tx.Unscoped().Where("id IN ?", stale).Delete(&models.RefreshSession{}).Error
		}
		return nil
	})
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// RotateRefreshSession swaps oldHash for newHash in the user's session that
// holds it and extends the session to expiresAt. oldHash is accepted if it is
// the session's current token, or the token rotated out less than
// refreshRotationGrace ago (two tabs refreshing at once, or a lost response).
// Each branch is a single conditional UPDATE, so concurrent rotations of the
// same token cannot both win the current-token branch. Returns ErrUnauthorized
// when no live session matches.
func (s *userService) RotateRefreshSession(userID, oldHash, newHash string, expiresAt time.Time) error {
	now := time.Now()

	res := s.db.Model(&models.RefreshSession{}).
		Where("user_id = ? AND token_hash = ? AND expires_at > ?", userID, oldHash, now).
		Updates(map[string]interface{}{
			"previous_token_hash": gorm.Expr("token_hash"),
			"token_hash":          newHash,
			"rotated_at":          now,
			"expires_at":          expiresAt,
		})
	if res.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}

	// Grace path: keep previous_token_hash as is so the window is measured
	// from the first rotation and cannot be extended by replaying the token.
	res = s.db.Model(&models.RefreshSession{}).
		Where("user_id = ? AND previous_token_hash = ? AND rotated_at > ? AND expires_at > ?",
			userID, oldHash, now.Add(-refreshRotationGrace), now).
		Updates(map[string]interface{}{
			"token_hash": newHash,
			"expires_at": expiresAt,
		})
	if res.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}
	return apperrors.ErrUnauthorized
}

// RevokeRefreshSession ends the session holding tokenHash (logout). Revoking a
// token that matches no session is not an error.
func (s *userService) RevokeRefreshSession(userID, tokenHash string) error {
	if err := s.db.Unscoped().
		Where("user_id = ? AND (token_hash = ? OR previous_token_hash = ?)", userID, tokenHash, tokenHash).
		Delete(&models.RefreshSession{}).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// ProfileSettingsUpdate lists the profile settings to change. A nil
// HideBalances leaves it as is; DefaultAccountID is only applied when
// SetDefaultAccount is true, and a nil DefaultAccountID then clears it.
type ProfileSettingsUpdate struct {
	HideBalances      *bool
	SetDefaultAccount bool
	DefaultAccountID  *string
}

// UpdateProfileSettings applies the given settings in a single update, so a
// request changing several of them succeeds or fails as a whole.
//
// A default account is the one preselected when the user records a new
// transaction. It must belong to the user, be active, and be a spending
// account (cash, credit card or debt).
func (s *userService) UpdateProfileSettings(userID string, update ProfileSettingsUpdate) error {
	changes := map[string]interface{}{}
	if update.HideBalances != nil {
		changes["hide_balances"] = *update.HideBalances
	}
	if update.SetDefaultAccount {
		if update.DefaultAccountID != nil {
			if err := s.validateDefaultAccount(userID, *update.DefaultAccountID); err != nil {
				return err
			}
		}
		changes["default_account_id"] = update.DefaultAccountID
	}
	if len(changes) == 0 {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "no profile settings to update")
	}

	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Updates(changes).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// validateDefaultAccount checks that accountID may be the user's default account.
func (s *userService) validateDefaultAccount(userID, accountID string) error {
	var account models.Account
	err := s.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrAccountNotFound
	}
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if !account.IsActive {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "default account must be active")
	}
	if !account.Type.IsSpending() {
		return apperrors.WithMessage(apperrors.ErrInvalidInput,
			"default account must be a cash, credit card or debt account")
	}
	return nil
}
