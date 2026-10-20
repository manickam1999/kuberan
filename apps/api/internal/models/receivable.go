package models

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ReceivableKind classifies why money is owed to the user. It is used for
// filtering and labels only; no behaviour depends on it (plan 021, D2a).
type ReceivableKind string

// Receivable kinds.
const (
	ReceivableKindSplit    ReceivableKind = "split"
	ReceivableKindOnBehalf ReceivableKind = "on_behalf"
	ReceivableKindClaim    ReceivableKind = "claim"
	ReceivableKindLoan     ReceivableKind = "loan"
	ReceivableKindDeposit  ReceivableKind = "deposit"
	ReceivableKindRefund   ReceivableKind = "refund"
)

// IsValidReceivableKind reports whether k is one of the supported kinds.
func IsValidReceivableKind(k ReceivableKind) bool {
	switch k {
	case ReceivableKindSplit, ReceivableKindOnBehalf, ReceivableKindClaim,
		ReceivableKindLoan, ReceivableKindDeposit, ReceivableKindRefund:
		return true
	}
	return false
}

// ReceivableStatus is the derived settlement state of a receivable.
type ReceivableStatus string

// Receivable statuses.
const (
	ReceivableStatusPending       ReceivableStatus = "pending"
	ReceivableStatusPartiallyPaid ReceivableStatus = "partially_paid"
	ReceivableStatusSettled       ReceivableStatus = "settled"
	ReceivableStatusWrittenOff    ReceivableStatus = "written_off"
)

// IsValidReceivableStatus reports whether s is one of the supported statuses.
func IsValidReceivableStatus(s ReceivableStatus) bool {
	switch s {
	case ReceivableStatusPending, ReceivableStatusPartiallyPaid,
		ReceivableStatusSettled, ReceivableStatusWrittenOff:
		return true
	}
	return false
}

// maxCounterpartyNameLen is the maximum length, in runes, of a normalized counterparty name.
const maxCounterpartyNameLen = 100

// Receivable is money owed to the user by a counterparty, attached to the
// expense that put the money out. All amounts are int64 cents.
type Receivable struct {
	Base
	UserID           string           `gorm:"type:uuid;not null" json:"user_id"`
	TransactionID    string           `gorm:"type:uuid;not null" json:"transaction_id"`
	CounterpartyName string           `gorm:"type:varchar(100);not null" json:"counterparty_name"`
	CounterpartyKey  string           `gorm:"type:varchar(100);not null" json:"counterparty_key"`
	Kind             ReceivableKind   `gorm:"type:varchar(20);not null;default:'split'" json:"kind"`
	Amount           int64            `gorm:"type:bigint;not null" json:"amount"`
	PaidAmount       int64            `gorm:"type:bigint;not null;default:0" json:"paid_amount"`
	WrittenOffAmount int64            `gorm:"type:bigint;not null;default:0" json:"written_off_amount"`
	Status           ReceivableStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`

	// Outstanding is derived (amount - paid - written off) and never stored.
	// Call ComputeOutstanding to populate it before serializing.
	Outstanding int64 `gorm:"-" json:"outstanding"`

	// Settlements are loaded on the detail endpoint only.
	Settlements []ReceivableSettlement `gorm:"foreignKey:ReceivableID" json:"settlements,omitempty"`

	// Transaction is the trimmed expense view attached by the service on the
	// receivables list and detail endpoints. It is not a GORM association.
	Transaction *ReceivableExpense `gorm:"-" json:"transaction,omitempty"`
}

// ReceivableExpense is the trimmed view of a receivable's parent expense.
type ReceivableExpense struct {
	ID             string    `json:"id"`
	Description    string    `json:"description"`
	Date           time.Time `json:"date"`
	Amount         int64     `json:"amount"`
	PersonalAmount *int64    `json:"personal_amount"`
	AccountID      string    `json:"account_id"`
	CategoryID     *string   `json:"category_id"`
}

// OutstandingAmount returns amount - paid_amount - written_off_amount.
func (r *Receivable) OutstandingAmount() int64 {
	return r.Amount - r.PaidAmount - r.WrittenOffAmount
}

// ComputeOutstanding refreshes the derived Outstanding field and returns it.
func (r *Receivable) ComputeOutstanding() int64 {
	r.Outstanding = r.OutstandingAmount()
	return r.Outstanding
}

// DeriveStatus returns the status implied by the receivable's amounts:
//   - pending: nothing paid or written off, outstanding > 0
//   - partially_paid: outstanding > 0 and something paid or written off
//   - settled: outstanding = 0 and nothing written off
//   - written_off: outstanding = 0 and something written off
func (r *Receivable) DeriveStatus() ReceivableStatus {
	outstanding := r.OutstandingAmount()
	switch {
	case outstanding > 0 && r.PaidAmount == 0 && r.WrittenOffAmount == 0:
		return ReceivableStatusPending
	case outstanding > 0:
		return ReceivableStatusPartiallyPaid
	case r.WrittenOffAmount > 0:
		return ReceivableStatusWrittenOff
	default:
		return ReceivableStatusSettled
	}
}

// ReceivableSettlement is a repayment against a receivable. TransactionID is
// the reimbursement transaction that credited AccountID.
type ReceivableSettlement struct {
	Base
	UserID        string    `gorm:"type:uuid;not null" json:"-"`
	ReceivableID  string    `gorm:"type:uuid;not null" json:"receivable_id"`
	TransactionID string    `gorm:"type:uuid;not null" json:"transaction_id"`
	AccountID     string    `gorm:"type:uuid;not null" json:"account_id"`
	Amount        int64     `gorm:"type:bigint;not null" json:"amount"`
	Date          time.Time `gorm:"not null" json:"date"`
}

// NormalizeCounterpartyName collapses internal whitespace in raw and returns the
// display name together with its lowercase grouping key. The name must be 1..100
// runes after normalization, otherwise an error is returned.
func NormalizeCounterpartyName(raw string) (name, key string, err error) {
	name = strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxCounterpartyNameLen {
		return "", "", fmt.Errorf("counterparty name must be between 1 and %d characters", maxCounterpartyNameLen)
	}
	return name, strings.ToLower(name), nil
}
