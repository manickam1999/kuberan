package models

import "time"

// ReminderKind says what a reminder is about.
type ReminderKind string

// Reminder kinds (plan 023, 5.6).
const (
	ReminderKindCardDue    ReminderKind = "card_due"
	ReminderKindLoanDue    ReminderKind = "loan_due"
	ReminderKindFDMaturity ReminderKind = "fd_maturity"
)

// Valid reports whether k is a known reminder kind.
func (k ReminderKind) Valid() bool {
	switch k {
	case ReminderKindCardDue, ReminderKindLoanDue, ReminderKindFDMaturity:
		return true
	default:
		return false
	}
}

// Reminder is one upcoming occurrence of a due date: a credit card statement,
// a loan instalment or a fixed-deposit maturity. The API keeps these rows in
// sync with accounts.due_day and fixed_deposit_details.maturity_date; the
// Telegram bot pulls the ones that are due and confirms each as sent.
type Reminder struct {
	Base
	UserID string       `gorm:"type:uuid;not null;index" json:"user_id"`
	Kind   ReminderKind `gorm:"type:varchar(16);not null;uniqueIndex:idx_reminders_occurrence,priority:1" json:"kind"`
	// RefID is the account the reminder is about.
	RefID string `gorm:"type:uuid;not null;uniqueIndex:idx_reminders_occurrence,priority:2" json:"ref_id"`
	// DueOn is the date (UTC midnight) the card, instalment or deposit falls due.
	DueOn time.Time `gorm:"type:date;not null;uniqueIndex:idx_reminders_occurrence,priority:3" json:"due_on"`
	// RemindAt is when the reminder becomes sendable.
	RemindAt time.Time  `gorm:"not null" json:"remind_at"`
	SentAt   *time.Time `json:"sent_at,omitempty"`
	// Enabled switches this account's reminder on or off.
	Enabled bool `gorm:"not null;default:true" json:"enabled"`
}
