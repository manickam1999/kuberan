package models

import (
	"time"

	"gorm.io/gorm"
)

// AccountType represents the type of account
type AccountType string

const (
	AccountTypeCash       AccountType = "cash"
	AccountTypeInvestment AccountType = "investment"
	AccountTypeDebt       AccountType = "debt"
	AccountTypeCreditCard AccountType = "credit_card"
)

// Account represents a financial account in the system
type Account struct {
	Base
	UserID      string      `gorm:"type:uuid;not null" json:"user_id"`
	Name        string      `gorm:"not null" json:"name"`
	Type        AccountType `gorm:"not null" json:"type"`
	Description string      `json:"description"`
	Balance     int64       `gorm:"type:bigint;not null;default:0" json:"balance"`
	Currency    string      `gorm:"not null;default:'USD'" json:"currency"`
	IsActive    bool        `gorm:"default:true" json:"is_active"`
	IsPinned    bool        `gorm:"default:false" json:"is_pinned"`

	// For investment accounts
	Broker        string       `json:"broker,omitempty"` // E.g., Robinhood, Fidelity, etc.
	AccountNumber string       `json:"account_number,omitempty"`
	Investments   []Investment `gorm:"foreignKey:AccountID" json:"investments,omitempty"`

	// For debt and credit card accounts
	InterestRate float64   `json:"interest_rate,omitempty"`
	DueDate      time.Time `json:"due_date,omitempty"`
	CreditLimit  int64     `gorm:"type:bigint;default:0" json:"credit_limit,omitempty"`

	// Relationships
	Transactions []Transaction `gorm:"foreignKey:AccountID" json:"transactions,omitempty"`
}

// BeforeCreate hook to set default values based on account type
func (a *Account) BeforeCreate(tx *gorm.DB) error {
	// Call Base BeforeCreate to generate UUID
	if err := a.Base.BeforeCreate(tx); err != nil {
		return err
	}

	switch a.Type {
	case AccountTypeCash:
		a.Broker = ""
		a.AccountNumber = ""
		a.InterestRate = 0
	case AccountTypeInvestment:
		a.InterestRate = 0
	case AccountTypeDebt:
		a.Broker = ""
		a.AccountNumber = ""
	case AccountTypeCreditCard:
		a.Broker = ""
		a.AccountNumber = ""
	}
	return nil
}
