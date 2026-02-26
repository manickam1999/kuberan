package services

import (
	"encoding/json"
	"kuberan/internal/logger"
	"kuberan/internal/models"

	"gorm.io/gorm"
)

// auditService handles audit log recording.
type auditService struct {
	db *gorm.DB
}

// NewAuditService creates a new AuditServicer.
func NewAuditService(db *gorm.DB) AuditServicer {
	return &auditService{db: db}
}

// Log records an audit event. Errors are logged but never propagate
// to avoid disrupting the main operation.
func (s *auditService) Log(userID, action, resourceType, resourceID, ipAddress string, changes map[string]any) {
	var changesJSON string
	if changes != nil {
		data, err := json.Marshal(changes)
		if err != nil {
			logger.Get().Errorw("failed to marshal audit log changes", "error", err, "action", action)
			changesJSON = "{}"
		} else {
			changesJSON = string(data)
		}
	}

	entry := &models.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    ipAddress,
		Changes:      changesJSON,
	}

	if err := s.db.Create(entry).Error; err != nil {
		logger.Get().Errorw("failed to create audit log entry",
			"error", err,
			"user_id", userID,
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID,
		)
	}
}
