package models

import (
	"time"

	"kuberan/internal/uuid"

	"gorm.io/gorm"
)

// PortfolioSnapshot represents a point-in-time snapshot of a user's net worth.
// This is immutable time-series data — no Base embed, no soft deletes.
type PortfolioSnapshot struct {
	ID                  string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID              string    `gorm:"type:uuid;not null" json:"user_id"`
	RecordedAt          time.Time `gorm:"not null" json:"recorded_at"`
	TotalNetWorth       int64     `gorm:"type:bigint;not null" json:"total_net_worth"`
	CashBalance         int64     `gorm:"type:bigint;not null" json:"cash_balance"`
	InvestmentValue     int64     `gorm:"type:bigint;not null" json:"investment_value"`
	DebtBalance         int64     `gorm:"type:bigint;not null" json:"debt_balance"`
	FixedDepositBalance int64     `gorm:"type:bigint;not null;default:0" json:"fixed_deposit_balance"`
}

// BeforeCreate hook generates a UUIDv7 for new records
func (p *PortfolioSnapshot) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New()
	}
	return nil
}
