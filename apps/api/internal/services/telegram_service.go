package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
)

const (
	linkCodeLength = 6
	linkCodeExpiry = 15 * time.Minute
)

// telegramService handles Telegram linking business logic.
type telegramService struct {
	db *gorm.DB
}

// NewTelegramService creates a new TelegramServicer.
func NewTelegramService(db *gorm.DB) TelegramServicer {
	return &telegramService{db: db}
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

	// Generate random link code
	code, err := generateRandomCode(linkCodeLength)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	expiresAt := time.Now().Add(linkCodeExpiry)

	if dbErr != nil {
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			// Create new link record with pending code
			link := &models.TelegramLink{
				UserID:            userID,
				LinkCode:          code,
				LinkCodeExpiresAt: &expiresAt,
				IsActive:          false,
			}

			if err := s.db.Create(link).Error; err != nil {
				return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
			}

			return link, nil
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, dbErr)
	}

	// Update existing link with new code
	existingLink.LinkCode = code
	existingLink.LinkCodeExpiresAt = &expiresAt

	if err := s.db.Save(&existingLink).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return &existingLink, nil
}

// CompleteLink completes the linking process by verifying the code
func (s *telegramService) CompleteLink(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error {
	var link models.TelegramLink
	if err := s.db.Where("link_code = ?", linkCode).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrInvalidLinkCode
		}
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	// Check if code has expired
	if link.LinkCodeExpiresAt == nil || time.Now().After(*link.LinkCodeExpiresAt) {
		return apperrors.ErrLinkCodeExpired
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
func (s *telegramService) GetUserWithAuthToken(telegramUserID int64) (map[string]interface{}, error) {
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

	return map[string]interface{}{
		"user_id":          user.ID,
		"email":            user.Email,
		"auth_token":       token,
		"default_currency": link.DefaultCurrency,
	}, nil
}

// generateRandomCode generates a random alphanumeric code of specified length
func generateRandomCode(length int) (string, error) {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	code := hex.EncodeToString(bytes)[:length]
	return code, nil
}
