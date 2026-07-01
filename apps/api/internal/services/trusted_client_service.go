package services

import (
	"errors"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// trustedClientService handles trust-on-first-use OAuth client business logic.
type trustedClientService struct {
	db *gorm.DB
}

// NewTrustedClientService creates a new TrustedClientServicer.
func NewTrustedClientService(db *gorm.DB) TrustedClientServicer {
	return &trustedClientService{db: db}
}

// IsTrusted reports whether the given OAuth client_id has been approved.
func (s *trustedClientService) IsTrusted(clientID string) (bool, error) {
	if clientID == "" {
		return false, apperrors.WithMessage(apperrors.ErrInvalidInput, "client_id is required")
	}

	var count int64
	if err := s.db.Model(&models.TrustedOAuthClient{}).
		Where("client_id = ?", clientID).
		Count(&count).Error; err != nil {
		return false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return count > 0, nil
}

// Trust records the given OAuth client as approved. It is idempotent: trusting
// an already-trusted client returns the existing record.
func (s *trustedClientService) Trust(clientID, name string) (*models.TrustedOAuthClient, error) {
	if clientID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "client_id is required")
	}

	var existing models.TrustedOAuthClient
	err := s.db.Where("client_id = ?", clientID).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	client := &models.TrustedOAuthClient{ClientID: clientID, Name: name}
	if err := s.db.Create(client).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return client, nil
}

// ListTrusted returns all trusted OAuth clients.
func (s *trustedClientService) ListTrusted() ([]models.TrustedOAuthClient, error) {
	var clients []models.TrustedOAuthClient
	if err := s.db.Order("created_at ASC").Find(&clients).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return clients, nil
}
