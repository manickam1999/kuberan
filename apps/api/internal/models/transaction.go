package models

import "time"

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeIncome     TransactionType = "income"
	TransactionTypeExpense    TransactionType = "expense"
	TransactionTypeTransfer   TransactionType = "transfer"
	TransactionTypeInvestment TransactionType = "investment"

	// TransactionTypeReimbursement is a repayment credited to an account for a
	// receivable. It is created only by the server (never accepted from clients),
	// is not income, and cannot be edited or deleted directly (plan 021).
	TransactionTypeReimbursement TransactionType = "reimbursement"
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

	// PersonalAmount is the user's own share of an expense, in cents. NULL means
	// the whole Amount is theirs. It is server-maintained (plan 021, D6) and is
	// always serialized (null when unset). Reports aggregate
	// COALESCE(personal_amount, amount).
	PersonalAmount *int64 `gorm:"type:bigint;check:transactions_personal_amount_check,personal_amount IS NULL OR (personal_amount >= 0 AND personal_amount <= amount)" json:"personal_amount"`

	// For transfers
	ToAccountID *string `gorm:"type:uuid" json:"to_account_id,omitempty"`

	// Relationships
	Account   Account   `gorm:"foreignKey:AccountID" json:"account"`
	ToAccount *Account  `gorm:"foreignKey:ToAccountID" json:"to_account,omitempty"`
	Category  *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`

	// Attachments are receipt images/PDFs. Preloaded only on detail queries
	// (not on list). See plans/017-transaction-receipts.
	Attachments []TransactionAttachment `gorm:"foreignKey:TransactionID" json:"attachments,omitempty"`

	// Receivables are the amounts owed back to the user for a shared expense.
	// Preloaded on list and detail queries for expenses (plan 021).
	Receivables []Receivable `gorm:"foreignKey:TransactionID" json:"receivables,omitempty"`

	// AttachmentsCount is a derived (non-persisted) count of receipts, populated
	// on list queries so the UI can show a paperclip indicator without loading
	// the attachment rows. See plans/017-transaction-receipts.
	AttachmentsCount int `gorm:"-" json:"attachments_count,omitempty"`
}
