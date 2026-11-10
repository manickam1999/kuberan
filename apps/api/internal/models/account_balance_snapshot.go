package models

import "time"

// AccountBalanceSnapshot is one account's balance on one UTC day, in the
// account's own currency. It is immutable time-series data written by the
// snapshot pipeline (and the first-run backfill), keyed by (AccountID, Day);
// there is no ID, no soft delete, and a later run on the same day overwrites
// the row.
type AccountBalanceSnapshot struct {
	AccountID string    `gorm:"type:uuid;primaryKey" json:"account_id"`
	Day       time.Time `gorm:"type:date;primaryKey" json:"day"`
	UserID    string    `gorm:"type:uuid;not null;index:idx_account_balance_snapshots_user_day,priority:1" json:"user_id"`
	Balance   int64     `gorm:"type:bigint;not null" json:"balance"`
	Currency  string    `gorm:"size:3;not null" json:"currency"`
}
