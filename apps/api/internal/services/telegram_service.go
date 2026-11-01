package services

import (
	"crypto/rand"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
)

const (
	linkCodeLength = 8
	linkCodeExpiry = 15 * time.Minute

	// linkCodeAlphabet is Crockford-style base32 minus the characters people
	// confuse when copying a code (0/O, 1/I/L) and U: 30 symbols, so an
	// 8-character code has ~39 bits of entropy.
	linkCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

	// maxLinkFailures failed CompleteLink attempts per Telegram user are
	// allowed per linkFailureWindow before further attempts are refused.
	maxLinkFailures   = 5
	linkFailureWindow = 15 * time.Minute

	// linkCodeInsertAttempts bounds retries on the (astronomically rare)
	// unique-index collision of a freshly generated code.
	linkCodeInsertAttempts = 3
)

// telegramService handles Telegram linking business logic.
type telegramService struct {
	db *gorm.DB
	// linkFailures counts failed link attempts per Telegram user ID. It is
	// in-memory (per process), which is enough to make guessing codes
	// through the bot impractical.
	linkFailures *middleware.RateLimiter
}

// NewTelegramService creates a new TelegramServicer.
func NewTelegramService(db *gorm.DB) TelegramServicer {
	return &telegramService{
		db:           db,
		linkFailures: middleware.NewRateLimiter(maxLinkFailures, linkFailureWindow),
	}
}

// NormalizeLinkCode uppercases a link code and strips spaces and dashes, so
// codes typed by hand still match.
func NormalizeLinkCode(code string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.ToUpper(strings.TrimSpace(code)))
}

// GetLinkByUserID retrieves a Telegram link by user ID
func (s *telegramService) GetLinkByUserID(userID string) (*models.TelegramLink, error) {
	var link models.TelegramLink
	if err := s.db.Where("user_id = ?", userID).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &link, nil
}

// GetLinkByTelegramID retrieves a Telegram link by Telegram user ID
func (s *telegramService) GetLinkByTelegramID(telegramUserID int64) (*models.TelegramLink, error) {
	var link models.TelegramLink
	if err := s.db.Where("telegram_user_id = ? AND is_active = ?", telegramUserID, true).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &link, nil
}

// GenerateLinkCode generates a new link code for a user
func (s *telegramService) GenerateLinkCode(userID string) (*models.TelegramLink, error) {
	// Check if user already has a link
	var existingLink models.TelegramLink
	dbErr := s.db.Where("user_id = ?", userID).First(&existingLink).Error
	if dbErr != nil && !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, dbErr)
	}

	var lastErr error
	for attempt := 0; attempt < linkCodeInsertAttempts; attempt++ {
		code, err := generateLinkCode(linkCodeLength)
		if err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		expiresAt := time.Now().Add(linkCodeExpiry)

		if dbErr != nil {
			// Create new link record with pending code
			link := &models.TelegramLink{
				UserID:            userID,
				LinkCode:          code,
				LinkCodeExpiresAt: &expiresAt,
				IsActive:          false,
			}
			lastErr = s.db.Create(link).Error
			if lastErr == nil {
				return link, nil
			}
		} else {
			// Update existing link with new code
			existingLink.LinkCode = code
			existingLink.LinkCodeExpiresAt = &expiresAt
			lastErr = s.db.Save(&existingLink).Error
			if lastErr == nil {
				return &existingLink, nil
			}
		}
		if !isUniqueViolation(lastErr) {
			break
		}
	}
	return nil, apperrors.Wrap(apperrors.ErrInternalServer, lastErr)
}

// linkFailureKey is the rate-limiter key for a Telegram user's link attempts.
func linkFailureKey(telegramUserID int64) string {
	return "tg:" + strconv.FormatInt(telegramUserID, 10)
}

// CompleteLink completes the linking process by verifying the code. A
// Telegram user that fails maxLinkFailures times within linkFailureWindow is
// refused (ErrTooManyLinkAttempts) until the window passes, so codes cannot be
// brute-forced through the bot.
func (s *telegramService) CompleteLink(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error {
	key := linkFailureKey(telegramUserID)
	if s.linkFailures.Exhausted(key) {
		return apperrors.ErrTooManyLinkAttempts
	}
	fail := func(err error) error {
		s.linkFailures.Allow(key)
		return err
	}

	var link models.TelegramLink
	if err := s.db.Where("link_code = ? AND link_code <> ''", NormalizeLinkCode(linkCode)).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fail(apperrors.ErrInvalidLinkCode)
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Check if code has expired
	if link.LinkCodeExpiresAt == nil || time.Now().After(*link.LinkCodeExpiresAt) {
		return fail(apperrors.ErrLinkCodeExpired)
	}

	// Check if this Telegram user is already linked to another account
	var existingLink models.TelegramLink
	err := s.db.Where("telegram_user_id = ? AND user_id != ?", telegramUserID, link.UserID).First(&existingLink).Error
	if err == nil {
		return apperrors.ErrTelegramAlreadyLinked
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Update link with Telegram user info
	link.TelegramUserID = telegramUserID
	link.TelegramUsername = username
	link.TelegramFirstName = firstName
	link.LinkCode = ""
	link.LinkCodeExpiresAt = nil
	link.IsActive = true
	if defaultCurrency != "" {
		link.DefaultCurrency = defaultCurrency
	}

	if err := s.db.Save(&link).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return nil
}

// UnlinkAccount unlinks a Telegram account from a user
func (s *telegramService) UnlinkAccount(userID string) error {
	result := s.db.Where("user_id = ?", userID).Delete(&models.TelegramLink{})
	if result.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, result.Error)
	}

	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// RecordActivity updates the last message timestamp and increments message count
func (s *telegramService) RecordActivity(telegramUserID int64) error {
	now := time.Now()
	result := s.db.Model(&models.TelegramLink{}).
		Where("telegram_user_id = ?", telegramUserID).
		Updates(map[string]interface{}{
			"last_message_at": now,
			"message_count":   gorm.Expr("message_count + 1"),
		})

	if result.Error != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, result.Error)
	}

	return nil
}

// IsLinked checks if a user has a linked Telegram account
func (s *telegramService) IsLinked(userID string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.TelegramLink{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error; err != nil {
		return false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return count > 0, nil
}

// GetUserWithAuthToken retrieves user info and generates a bot auth token
func (s *telegramService) GetUserWithAuthToken(telegramUserID int64) (*TelegramUserAuth, error) {
	link, err := s.GetLinkByTelegramID(telegramUserID)
	if err != nil {
		return nil, err
	}

	// Get user
	var user models.User
	if err := s.db.Where("id = ?", link.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Generate bot token (long-lived JWT)
	token, err := middleware.GenerateBotToken(&user)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return &TelegramUserAuth{
		UserID:          user.ID,
		Email:           user.Email,
		AuthToken:       token,
		DefaultCurrency: link.DefaultCurrency,
	}, nil
}

// generateLinkCode returns a uniformly random code of length symbols from
// linkCodeAlphabet, using crypto/rand with rejection sampling (no modulo bias).
func generateLinkCode(length int) (string, error) {
	const alphabetLen = len(linkCodeAlphabet)
	// Largest multiple of alphabetLen that fits in a byte.
	const limit = 256 - 256%alphabetLen

	out := make([]byte, 0, length)
	buf := make([]byte, length*2)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, linkCodeAlphabet[int(b)%alphabetLen])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}
