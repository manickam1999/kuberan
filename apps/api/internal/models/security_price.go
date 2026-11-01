package models

import (
	"time"

	"kuberan/internal/uuid"

	"gorm.io/gorm"
)

// SecurityPrice represents a historical price entry for a security.
// This is immutable time-series data — no Base embed, no soft deletes.
type SecurityPrice struct {
	ID         string    `gorm:"type:uuid;primaryKey" json:"id"`
	SecurityID string    `gorm:"type:uuid;not null;uniqueIndex:uq_security_prices_security_recorded,priority:1" json:"security_id"`
	Price      int64     `gorm:"type:bigint;not null" json:"price"`
	RecordedAt time.Time `gorm:"not null;uniqueIndex:uq_security_prices_security_recorded,priority:2" json:"recorded_at"`
	Security   Security  `gorm:"foreignKey:SecurityID" json:"security,omitempty"`
}

// BeforeCreate hook generates a UUIDv7 for new records
func (s *SecurityPrice) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New()
	}
	return nil
}
