package models

import (
	"time"

	"kuberan/internal/uuid"

	"gorm.io/gorm"
)

// DemoRow records one row created by the demo household (plan 023 5.7). The
// demo is removed by hard-deleting exactly the rows listed here.
type DemoRow struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:uq_demo_rows_user_table_row,priority:1" json:"user_id"`
	Table     string    `gorm:"column:table_name;size:64;not null;uniqueIndex:uq_demo_rows_user_table_row,priority:2" json:"table_name"`
	RowID     string    `gorm:"type:uuid;not null;uniqueIndex:uq_demo_rows_user_table_row,priority:3" json:"row_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the table name for DemoRow.
func (DemoRow) TableName() string { return "demo_rows" }

// BeforeCreate generates a UUIDv7 for new records.
func (d *DemoRow) BeforeCreate(_ *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New()
	}
	return nil
}
