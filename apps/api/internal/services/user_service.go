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
	fx FXServicer
}

// NewUserService creates a new UserServicer. fxService validates the currency
// a user picks as their base currency.
func NewUserService(db *gorm.DB, fxService FXServicer) UserServicer {
	return &userService{db: db, fx: fxService}
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

// CreateUserOption configures optional settings of CreateUser.
type CreateUserOption func(*createUserOptions)

type createUserOptions struct {
	baseCurrency string
}

// WithBaseCurrency sets the new user's reporting currency. It is checked like
// a PATCH /profile base_currency change (ErrUnsupportedCurrency when it is not
// usable). An empty code keeps the column default.
func WithBaseCurrency(code string) CreateUserOption {
	return func(o *createUserOptions) { o.baseCurrency = code }
}

// checkBaseCurrencySupported reports ErrUnsupportedCurrency unless code (already
// normalised) is a currency that can be a base currency. Shared by sign-up and
// PATCH /profile so both use one rule.
func (s *userService) checkBaseCurrencySupported(code string) error {
	if !isCurrencyCode(code) || !s.fx.IsSupportedCurrency(code, time.Now()) {
		return apperrors.ErrUnsupportedCurrency
	}
	return nil
}

// CreateUser registers a new user
func (s *userService) CreateUser(email, password, firstName, lastName string, opts ...CreateUserOption) (*models.User, error) {
	var o createUserOptions
	for _, opt := range opts {
		opt(&o)
	}
	baseCurrency := normalizeCode(o.baseCurrency)
	email = NormalizeEmail(email)
	// Validate input
	if email == "" || password == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "email and password are required")
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if baseCurrency != "" {
		if err := s.checkBaseCurrencySupported(baseCurrency); err != nil {
			return nil, err
		}
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
	if baseCurrency != "" {
		user.BaseCurrency = baseCurrency
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
// hash of its first refresh token and returns its ID (the access token's sid
// claim). Expired sessions for the user are pruned, and the oldest are dropped
// beyond maxRefreshSessionsPerUser, so the table stays bounded even if clients
// never log out.
func (s *userService) CreateRefreshSession(userID, tokenHash, userAgent, ipAddress string, expiresAt time.Time) (string, error) {
	var sessionID string
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
		sessionID = session.ID

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
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return sessionID, nil
}

// RotateRefreshSession swaps oldHash for newHash in the user's session that
// holds it, extends the session to expiresAt and returns the session's ID.
// oldHash is accepted if it is the session's current token, or the token
// rotated out less than refreshRotationGrace ago (two tabs refreshing at once,
// or a lost response). Each branch is a single conditional UPDATE, so
// concurrent rotations of the same token cannot both win the current-token
// branch. Returns ErrUnauthorized when no live session matches.
func (s *userService) RotateRefreshSession(userID, oldHash, newHash string, expiresAt time.Time) (string, error) {
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
		return "", apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 0 {
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
			return "", apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
		}
	}
	if res.RowsAffected != 1 {
		return "", apperrors.ErrUnauthorized
	}

	var session models.RefreshSession
	if err := s.db.Select("id").Where("user_id = ? AND token_hash = ?", userID, newHash).
		First(&session).Error; err != nil {
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return session.ID, nil
}

// ListRefreshSessions returns the user's live device sessions (not revoked, not
// expired), most recently active first.
func (s *userService) ListRefreshSessions(userID string) ([]models.RefreshSession, error) {
	var sessions []models.RefreshSession
	if err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("COALESCE(rotated_at, created_at) DESC").Find(&sessions).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return sessions, nil
}

// RevokeRefreshSessionByID ends one of the user's other device sessions. The
// current session cannot be revoked here (ErrCurrentSession; use logout), and
// an unknown, foreign or already-ended session is ErrSessionNotFound.
func (s *userService) RevokeRefreshSessionByID(userID, sessionID, currentSessionID string) error {
	if sessionID == currentSessionID && currentSessionID != "" {
		return apperrors.ErrCurrentSession
	}
	res := s.db.Unscoped().Where("id = ? AND user_id = ? AND expires_at > ?", sessionID, userID, time.Now()).
		Delete(&models.RefreshSession{})
	if res.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}
	return nil
}

// RevokeOtherRefreshSessions ends every session of the user except
// currentSessionID and returns how many ended. currentSessionID must be
// non-empty: without it there is no way to tell which session to keep.
func (s *userService) RevokeOtherRefreshSessions(userID, currentSessionID string) (int64, error) {
	if currentSessionID == "" {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput,
			"current session is unknown; sign in again before revoking other devices")
	}
	return s.revokeSessionsExcept(s.db, userID, currentSessionID)
}

// revokeSessionsExcept hard-deletes the user's sessions other than keepID
// (every session when keepID is empty).
func (s *userService) revokeSessionsExcept(db *gorm.DB, userID, keepID string) (int64, error) {
	q := db.Unscoped().Where("user_id = ?", userID)
	if keepID != "" {
		q = q.Where("id <> ?", keepID)
	}
	res := q.Delete(&models.RefreshSession{})
	if res.Error != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, res.Error)
	}
	return res.RowsAffected, nil
}

// checkCurrentPassword verifies a password re-entered for a sensitive change.
// A wrong password counts toward the same lockout as login failures; while the
// account is locked every attempt is refused with ACCOUNT_LOCKED.
func (s *userService) checkCurrentPassword(user *models.User, password string) error {
	now := time.Now()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return apperrors.ErrAccountLocked
	}
	if len(password) <= MaxPasswordBytes && s.VerifyPassword(user, password) {
		return nil
	}
	triggered, err := s.recordFailedLogin(user.ID, now)
	if err != nil {
		return err
	}
	if triggered {
		return apperrors.ErrAccountLocked
	}
	return apperrors.ErrInvalidCurrentPassword
}

// ChangePassword sets a new password after verifying the current one (wrong
// attempts count toward the login lockout), then revokes every refresh session
// except keepSessionID in the same transaction. It returns the number of
// sessions revoked. With an empty keepSessionID all sessions end.
func (s *userService) ChangePassword(userID, currentPassword, newPassword, keepSessionID string) (int64, error) {
	if err := ValidatePassword(newPassword); err != nil {
		return 0, err
	}
	user, err := s.GetUserByID(userID)
	if err != nil {
		return 0, err
	}
	if err := s.checkCurrentPassword(user, currentPassword); err != nil {
		return 0, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return 0, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var revoked int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"password":              string(hashed),
			"failed_login_attempts": 0,
			"locked_until":          nil,
		}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		n, err := s.revokeSessionsExcept(tx, userID, keepSessionID)
		revoked = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return revoked, nil
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
// SetDefaultAccount is true, and a nil DefaultAccountID then clears it. A nil
// BaseCurrency leaves the base currency as is.
//
// FirstName and LastName replace the names when non-nil. A non-nil Email
// replaces the address (trimmed and lower-cased like registration) and needs
// CurrentPassword, unless it equals the current address, in which case it is
// ignored.
type ProfileSettingsUpdate struct {
	HideBalances      *bool
	SetDefaultAccount bool
	DefaultAccountID  *string
	BaseCurrency      *string
	FirstName         *string
	LastName          *string
	Email             *string
	CurrentPassword   string
}

// UpdateProfileSettings applies the given settings in a single update, so a
// request changing several of them succeeds or fails as a whole.
//
// A default account is the one preselected when the user records a new
// transaction. It must belong to the user, be active, and be a spending
// account (cash, credit card or debt).
//
// The base currency is upper-cased and must be supported (see
// FXServicer.IsSupportedCurrency) unless it equals the user's current one. A
// different base is refused with BASE_CURRENCY_LOCKED once the user has any
// investment transaction (cost basis is frozen in the base currency).
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
	if update.BaseCurrency != nil {
		code, err := s.validateBaseCurrency(userID, *update.BaseCurrency)
		if err != nil {
			return err
		}
		changes["base_currency"] = code
	}
	if update.FirstName != nil {
		changes["first_name"] = strings.TrimSpace(*update.FirstName)
	}
	if update.LastName != nil {
		changes["last_name"] = strings.TrimSpace(*update.LastName)
	}
	if update.Email != nil {
		email, changed, err := s.validateEmailChange(userID, *update.Email, update.CurrentPassword)
		if err != nil {
			return err
		}
		if changed {
			changes["email"] = email
		}
	}
	if len(changes) == 0 {
		if update.Email != nil {
			return nil // the address is unchanged: nothing to write
		}
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "no profile settings to update")
	}

	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Updates(changes).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.ErrDuplicateEmail
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// validateEmailChange normalises newEmail and, when it differs from the user's
// current address, checks the current password and that no other user (even a
// soft-deleted one: the unique index covers them) holds it.
func (s *userService) validateEmailChange(userID, newEmail, currentPassword string) (email string, changed bool, err error) {
	email = NormalizeEmail(newEmail)
	if email == "" {
		return "", false, apperrors.WithMessage(apperrors.ErrInvalidInput, "email must not be empty")
	}
	user, err := s.GetUserByID(userID)
	if err != nil {
		return "", false, err
	}
	if email == user.Email {
		return email, false, nil
	}
	if currentPassword == "" {
		return "", false, apperrors.WithMessage(apperrors.ErrInvalidInput, "current_password is required to change email")
	}
	if err := s.checkCurrentPassword(user, currentPassword); err != nil {
		return "", false, err
	}
	var count int64
	if err := s.db.Unscoped().Model(&models.User{}).Where("email = ? AND id <> ?", email, userID).
		Count(&count).Error; err != nil {
		return "", false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if count > 0 {
		return "", false, apperrors.ErrDuplicateEmail
	}
	return email, true, nil
}

// validateBaseCurrency normalises code and checks that it may become the
// user's base currency: supported, or already the user's current one.
func (s *userService) validateBaseCurrency(userID, code string) (string, error) {
	code = normalizeCode(code)
	var user models.User
	if err := s.db.Select("id", "base_currency").Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", apperrors.ErrUserNotFound
		}
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if !isCurrencyCode(code) {
		return "", apperrors.ErrUnsupportedCurrency
	}
	if code == user.BaseCurrency {
		return code, nil
	}
	if err := s.checkBaseCurrencySupported(code); err != nil {
		return "", err
	}
	// Investment cost basis and realized gains are frozen in the base currency
	// at trade time, so they cannot be re-labelled by changing it.
	var history int64
	if err := s.db.Table("investment_transactions").
		Joins("JOIN investments ON investments.id = investment_transactions.investment_id AND investments.deleted_at IS NULL").
		Joins("JOIN accounts ON accounts.id = investments.account_id").
		Where("accounts.user_id = ? AND investment_transactions.deleted_at IS NULL", userID).
		Limit(1).Count(&history).Error; err != nil {
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if history > 0 {
		return "", apperrors.ErrBaseCurrencyLocked
	}
	return code, nil
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
