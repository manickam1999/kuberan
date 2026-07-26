package services

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute
)

// userService handles user-related business logic.
type userService struct {
	db *gorm.DB
}

// NewUserService creates a new UserServicer.
func NewUserService(db *gorm.DB) UserServicer {
	return &userService{db: db}
}

// CreateUser registers a new user
func (s *userService) CreateUser(email, password, firstName, lastName string) (*models.User, error) {
	// Validate input
	if email == "" || password == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "email and password are required")
	}

	// Check if user with email exists
	var count int64
	s.db.Model(&models.User{}).Where("email = ?", email).Count(&count)
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
		Email:     strings.ToLower(email),
		Password:  string(hashedPassword),
		FirstName: firstName,
		LastName:  lastName,
		IsActive:  true,
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *userService) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("email = ? AND is_active = ?", strings.ToLower(email), true).First(&user).Error; err != nil {
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
func (s *userService) AttemptLogin(email, password string) (*models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, apperrors.ErrInvalidCredentials
	}

	// Check if account is locked
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, apperrors.ErrAccountLocked
	}

	// Verify password
	if !s.VerifyPassword(user, password) {
		user.FailedLoginAttempts++
		if user.FailedLoginAttempts >= maxFailedAttempts {
			lockUntil := time.Now().Add(lockoutDuration)
			user.LockedUntil = &lockUntil
		}
		s.db.Save(user)
		return nil, apperrors.ErrInvalidCredentials
	}

	// Successful login: reset failed attempts
	now := time.Now()
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &now
	s.db.Save(user)

	return user, nil
}

// StoreRefreshTokenHash stores the hash of a refresh token for the given user.
func (s *userService) StoreRefreshTokenHash(userID, tokenHash string) error {
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Update("refresh_token_hash", tokenHash).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// GetRefreshTokenHash returns the stored refresh token hash for the given user.
func (s *userService) GetRefreshTokenHash(userID string) (string, error) {
	var user models.User
	if err := s.db.Select("refresh_token_hash").Where("id = ?", userID).First(&user).Error; err != nil {
		return "", apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return user.RefreshTokenHash, nil
}

// UpdateHideBalances sets the user's dashboard balance-visibility preference.
func (s *userService) UpdateHideBalances(userID string, hide bool) error {
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Update("hide_balances", hide).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}
