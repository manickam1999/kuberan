package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
)

// SystemStatus reports when the data pipelines and the backup last ran. A nil
// field means "never" (or "not available").
type SystemStatus struct {
	LatestPriceAt    *time.Time `json:"latest_price_at"`
	LatestFXRateAt   *time.Time `json:"latest_fx_rate_at"`
	LatestSnapshotAt *time.Time `json:"latest_snapshot_at"`
	LastBackupAt     *time.Time `json:"last_backup_at"`
}

// SystemStatusServicer reads freshness timestamps for the host-status chip.
type SystemStatusServicer interface {
	// GetStatus returns the status for userID. Prices, FX rates and backups are
	// global; the snapshot timestamp is the user's own latest snapshot.
	GetStatus(userID string) (*SystemStatus, error)
}

type systemStatusService struct {
	db *gorm.DB
}

// NewSystemStatusService creates a SystemStatusServicer.
func NewSystemStatusService(db *gorm.DB) SystemStatusServicer {
	return &systemStatusService{db: db}
}

// latestTime returns the newest value of column among rows of model matching
// the optional condition, or nil when there are none.
func (s *systemStatusService) latestTime(model any, column, query string, args ...any) (*time.Time, error) {
	tx := s.db.Model(model)
	if query != "" {
		tx = tx.Where(query, args...)
	}
	var rows []time.Time
	if err := tx.Order(column+" DESC").Limit(1).Pluck(column, &rows).Error; err != nil {
		return nil, fmt.Errorf("latest %s: %w", column, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	t := rows[0].UTC()
	return &t, nil
}

// GetStatus implements SystemStatusServicer.
func (s *systemStatusService) GetStatus(userID string) (*SystemStatus, error) {
	var (
		st  SystemStatus
		err error
	)
	if st.LatestPriceAt, err = s.latestTime(&models.SecurityPrice{}, "recorded_at", ""); err != nil {
		return nil, err
	}
	if st.LatestFXRateAt, err = s.latestTime(&models.FXRate{}, "recorded_at", ""); err != nil {
		return nil, err
	}
	if st.LatestSnapshotAt, err = s.latestTime(&models.PortfolioSnapshot{}, "recorded_at", "user_id = ?", userID); err != nil {
		return nil, err
	}
	if st.LastBackupAt, err = s.latestTime(&models.BackupRun{}, "finished_at", "status = ?", "ok"); err != nil {
		return nil, err
	}
	return &st, nil
}
