package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
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
	Kind             ReceivableKind   `gorm:"type:varchar(20);not null;default:'split';check:receivables_kind_check,kind IN ('split', 'on_behalf', 'claim', 'loan', 'deposit', 'refund')" json:"kind"`
	Amount           int64            `gorm:"type:bigint;not null;check:receivables_amount_check,amount > 0" json:"amount"`
	PaidAmount       int64            `gorm:"type:bigint;not null;default:0;check:receivables_paid_amount_check,paid_amount >= 0" json:"paid_amount"`
	WrittenOffAmount int64            `gorm:"type:bigint;not null;default:0;check:receivables_settled_within_amount_check,written_off_amount >= 0 AND paid_amount + written_off_amount <= amount" json:"written_off_amount"`
	Status           ReceivableStatus `gorm:"type:varchar(20);not null;default:'pending';check:receivables_status_check,status IN ('pending', 'partially_paid', 'settled', 'written_off')" json:"status"`

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
	Amount        int64     `gorm:"type:bigint;not null;check:receivable_settlements_amount_check,amount > 0" json:"amount"`
	Date          time.Time `gorm:"not null" json:"date"`

	// WrittenOffAmount is the amount written off by the same settle call
	// (write_off_remainder). Deleting the settlement reverses it together with
	// the payment. Zero for a plain settlement.
	WrittenOffAmount int64 `gorm:"type:bigint;not null;default:0;check:receivable_settlements_written_off_amount_check,written_off_amount >= 0" json:"written_off_amount"`
}

// NormalizeCounterpartyKey returns the grouping key for a raw counterparty name
// or a user-supplied key: whitespace is collapsed and the result lowercased. It
// is total (never fails) and maps the empty or all-whitespace string to "".
//
// Whitespace means unicode.IsSpace, because the implementation is
// strings.Join(strings.Fields(raw), " "): runs of spaces, tabs, newlines and
// other Unicode space characters (such as U+00A0 and U+0085) become one ASCII
// space, and leading and trailing space is dropped. The lowercasing is
// strings.ToLower (Unicode simple case mapping). The web client mirrors these
// semantics, so change them in both places together.
func NormalizeCounterpartyKey(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

// NormalizeCounterpartyName collapses whitespace in raw (see
// NormalizeCounterpartyKey for the exact semantics) and returns the display name
// together with its lowercase grouping key. The name must be 1..100 runes after
// normalization and contain no control characters (such as NUL, which
// PostgreSQL rejects) and only valid UTF-8, otherwise an error is returned.
func NormalizeCounterpartyName(raw string) (name, key string, err error) {
	name = strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxCounterpartyNameLen {
		return "", "", fmt.Errorf("counterparty name must be between 1 and %d characters", maxCounterpartyNameLen)
	}
	if !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", "", errors.New("counterparty name must not contain control characters")
	}
	return name, strings.ToLower(name), nil
}
