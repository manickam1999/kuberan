package models

import "time"

// BackupRun is one backup attempt, inserted by the backup service (apps/backup)
// after each pg_dump. The API only reads it, to report the last good backup.
type BackupRun struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	StartedAt  time.Time `gorm:"not null" json:"started_at"`
	FinishedAt time.Time `gorm:"not null;index:idx_backup_runs_status_finished,priority:2" json:"finished_at"`
	Status     string    `gorm:"size:10;not null;index:idx_backup_runs_status_finished,priority:1" json:"status"` // "ok" or "failed"
	SizeBytes  *int64    `gorm:"type:bigint" json:"size_bytes,omitempty"`
	Error      string    `gorm:"not null;default:''" json:"error"`
}

// TableName returns the table name for BackupRun.
func (BackupRun) TableName() string { return "backup_runs" }
