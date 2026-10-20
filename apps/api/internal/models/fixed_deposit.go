package models

import "time"

// PayoutType controls when interest is disbursed.
type PayoutType string

const (
	// PayoutTypeCumulative accumulates interest and pays it in a lump sum at maturity.
	PayoutTypeCumulative PayoutType = "cumulative"
	// PayoutTypePeriodic pays interest to a linked cash account on a regular schedule.
	PayoutTypePeriodic PayoutType = "periodic"
)

// PayoutFrequency is the interval for periodic interest payouts.
// Only relevant when PayoutType == PayoutTypePeriodic.
type PayoutFrequency string

const (
	PayoutFrequencyMonthly    PayoutFrequency = "monthly"
	PayoutFrequencyQuarterly  PayoutFrequency = "quarterly"
	PayoutFrequencySemiannual PayoutFrequency = "semiannual"
	PayoutFrequencyAnnual     PayoutFrequency = "annual"
)

// Compounding is the compounding frequency used to compute the projected maturity value.
type Compounding string

const (
	CompoundingMonthly    Compounding = "monthly"
	CompoundingQuarterly  Compounding = "quarterly"
	CompoundingSemiannual Compounding = "semiannual"
	CompoundingAnnual     Compounding = "annual"
)

// FixedDepositStatus is the lifecycle state of a fixed deposit.
type FixedDepositStatus string

const (
	FixedDepositStatusActive  FixedDepositStatus = "active"
	FixedDepositStatusMatured FixedDepositStatus = "matured"
	FixedDepositStatusClosed  FixedDepositStatus = "closed"
)

// FixedDepositDetails holds the term and payout metadata for a fixed_deposit account.
// It is a 1:1 extension of the Account row; rate lives on accounts.interest_rate,
// and the running balance lives on accounts.balance (= principal while active, 0 once closed).
type FixedDepositDetails struct {
	Base

	// AccountID is the FK to the parent fixed_deposit account (unique 1:1).
	AccountID string `gorm:"type:uuid;not null;uniqueIndex" json:"account_id"`

	// Principal is the amount locked in at opening, in cents.
	Principal int64 `gorm:"type:bigint;not null" json:"principal"`

	// StartDate is the date the deposit was opened.
	StartDate time.Time `gorm:"type:date;not null" json:"start_date"`

	// MaturityDate is the date the deposit matures (must be after StartDate).
	MaturityDate time.Time `gorm:"type:date;not null" json:"maturity_date"`

	// PayoutType determines whether interest is paid periodically or in a lump sum at maturity.
	PayoutType PayoutType `gorm:"type:varchar(16);not null" json:"payout_type"`

	// PayoutFrequency is required when PayoutType == PayoutTypePeriodic.
	PayoutFrequency *PayoutFrequency `gorm:"type:varchar(16)" json:"payout_frequency,omitempty"`

	// Compounding is the compounding frequency used for the projected maturity value calculation.
	Compounding Compounding `gorm:"type:varchar(16);not null" json:"compounding"`

	// LinkedAccountID is the cash account that receives periodic interest payouts
	// and is the default realization destination at close. Null for cumulative-only FDs.
	LinkedAccountID *string `gorm:"type:uuid" json:"linked_account_id,omitempty"`

	// NextPayoutDate is the next date on which a periodic payout should be posted.
	// The scheduled job advances this atomically after each posting (idempotency gate).
	NextPayoutDate *time.Time `gorm:"type:date" json:"next_payout_date,omitempty"`

	// MaturityValue is the projected value at maturity in cents, computed and stored at create
	// (and recomputed on any edit to rate/maturity/compounding). Treated as a projection;
	// the realized amount at close is authoritative.
	MaturityValue int64 `gorm:"type:bigint;not null;default:0" json:"maturity_value"`

	// Status is the lifecycle state of the deposit.
	Status FixedDepositStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
}
