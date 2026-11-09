package services

import (
	"encoding/json"
	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/uuid"

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
//
// user_id and resource_id are UUID columns. An empty ID is stored as NULL; a
// non-empty ID that is not a UUID (e.g. a non-UUID OAuth client_id) is stored
// as NULL with the raw value kept in changes under "user_ref" or
// "resource_ref", so the event is never dropped by a type error.
func (s *auditService) Log(userID, action, resourceType, resourceID, ipAddress string, changes map[string]any, opts ...LogOpts) {
	var opt LogOpts
	if len(opts) > 0 {
		opt = opts[0]
	}
	source := opt.Source
	if source == "" {
		source = models.AuditSourceWeb
	}
	userRef, userRaw := auditUUID(userID)
	resourceRef, resourceRaw := auditUUID(resourceID)
	if userRaw != "" || resourceRaw != "" {
		merged := make(map[string]any, len(changes)+2)
		for k, v := range changes {
			merged[k] = v
		}
		if userRaw != "" {
			merged["user_ref"] = userRaw
		}
		if resourceRaw != "" {
			merged["resource_ref"] = resourceRaw
		}
		changes = merged
	}

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
		UserID:       userRef,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceRef,
		IPAddress:    ipAddress,
		Changes:      changesJSON,
		Source:       source,
		ClientID:     opt.ClientID,
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

// auditUUID maps an ID to a UUID column value: nil for "", the canonical form
// for a valid UUID, and nil plus the raw value for anything else.
func auditUUID(id string) (ref *string, raw string) {
	if id == "" {
		return nil, ""
	}
	canonical, err := uuid.Parse(id)
	if err != nil {
		return nil, id
	}
	return &canonical, ""
}
