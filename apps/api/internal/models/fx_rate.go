package models

import (
	"time"

	"kuberan/internal/fx"
	"kuberan/internal/uuid"

	"gorm.io/gorm"
)

// FXRate is one observation of the exchange rate for a currency pair: one unit
// of Base is worth Rate units of Quote at RecordedAt. This is immutable
// time-series data — no Base embed, no soft deletes.
type FXRate struct {
	ID         string    `gorm:"type:uuid;primaryKey" json:"id"`
	Base       string    `gorm:"type:varchar(3);not null;uniqueIndex:uq_fx_rates_pair_recorded,priority:1" json:"base"`
	Quote      string    `gorm:"type:varchar(3);not null;uniqueIndex:uq_fx_rates_pair_recorded,priority:2" json:"quote"`
	Rate       fx.Rate   `gorm:"type:numeric(20,10);not null" json:"rate"`
	RecordedAt time.Time `gorm:"not null;uniqueIndex:uq_fx_rates_pair_recorded,priority:3" json:"recorded_at"`
	Source     string    `gorm:"type:varchar(32);not null;default:'yahoo'" json:"source"`
}

// TableName returns the table name for FXRate.
func (FXRate) TableName() string { return "fx_rates" }

// BeforeCreate hook generates a UUIDv7 for new records.
func (f *FXRate) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New()
	}
	return nil
}
