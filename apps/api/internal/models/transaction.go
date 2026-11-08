package models

import (
	"time"

	"gorm.io/gorm"

	"kuberan/internal/fx"
)

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

	// TransactionTypeOpeningBalance is an account's starting balance, written
	// when the account is created. It moves the balance but is not income or
	// spending, so every income/expense figure excludes it. Clients can edit
	// its amount, date and description, or delete it, but never create one.
	TransactionTypeOpeningBalance TransactionType = "opening_balance"
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

	// ToAmount is the amount credited to the destination account, in the
	// destination's currency, for a cross-currency transfer. NULL means the
	// destination receives Amount. Use EffectiveToAmount to read it.
	ToAmount *int64 `gorm:"type:bigint;check:transactions_to_amount_check,to_amount IS NULL OR to_amount > 0" json:"to_amount,omitempty"`

	// ImpliedRate is the response-only rate to_amount/amount (decimal string,
	// 10 dp, half-even), set for cross-currency transfers.
	ImpliedRate string `gorm:"-" json:"implied_rate,omitempty"`

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

// EffectiveToAmount is what a transfer credits to its destination account:
// ToAmount when set, otherwise Amount.
func (t *Transaction) EffectiveToAmount() int64 {
	if t.ToAmount != nil {
		return *t.ToAmount
	}
	return t.Amount
}

// FillImpliedRate sets the response-only ImpliedRate from Amount and ToAmount.
// It is a no-op unless the transaction carries a to_amount.
func (t *Transaction) FillImpliedRate() {
	t.ImpliedRate = ""
	if t.ToAmount == nil {
		return
	}
	if r, err := fx.RateFromAmounts(t.Amount, *t.ToAmount); err == nil {
		t.ImpliedRate = r.String()
	}
}

// AfterFind populates the response-only fields.
func (t *Transaction) AfterFind(_ *gorm.DB) error {
	t.FillImpliedRate()
	return nil
}
