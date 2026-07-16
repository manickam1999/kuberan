package models

import "time"

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeIncome     TransactionType = "income"
	TransactionTypeExpense    TransactionType = "expense"
	TransactionTypeTransfer   TransactionType = "transfer"
	TransactionTypeInvestment TransactionType = "investment"
)

// Transaction represents a financial transaction in the system
type Transaction struct {
	Base
	UserID      string          `gorm:"type:uuid;not null" json:"user_id"`
	AccountID   string          `gorm:"type:uuid;not null" json:"account_id"`
	CategoryID  *string         `gorm:"type:uuid" json:"category_id,omitempty"`
	Type        TransactionType `gorm:"not null" json:"type"`
	Amount      int64           `gorm:"type:bigint;not null" json:"amount"`
	Description string          `json:"description"`
	Date        time.Time       `gorm:"not null" json:"date"`

	// For transfers
	ToAccountID *string `gorm:"type:uuid" json:"to_account_id,omitempty"`

	// Relationships
	Account   Account   `gorm:"foreignKey:AccountID" json:"account"`
	ToAccount *Account  `gorm:"foreignKey:ToAccountID" json:"to_account,omitempty"`
	Category  *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`

	// Attachments are receipt images/PDFs. Preloaded only on detail queries
	// (not on list). See plans/017-transaction-receipts.
	Attachments []TransactionAttachment `gorm:"foreignKey:TransactionID" json:"attachments,omitempty"`
}
