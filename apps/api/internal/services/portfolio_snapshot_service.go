package services

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// portfolioSnapshotService handles portfolio snapshot operations.
type portfolioSnapshotService struct {
	db *gorm.DB
}

// NewPortfolioSnapshotService creates a new PortfolioSnapshotServicer.
func NewPortfolioSnapshotService(db *gorm.DB) PortfolioSnapshotServicer {
	return &portfolioSnapshotService{db: db}
}

// snapshotUpsertBatchSize bounds the rows per multi-row snapshot upsert.
const snapshotUpsertBatchSize = 500

// ComputeAndRecordSnapshots computes and stores a net worth snapshot for all
// active users and returns how many were recorded.
//
// The computation is set-based: one GROUP BY (user_id, type) query for account
// balances, one query for all holdings in active investment accounts and one
// batched latest-price lookup, regardless of the number of users. Snapshots
// are upserted on (user_id, recorded_at) in multi-row batches; when a batch
// fails it is retried row by row so one bad user cannot block the rest. Rows
// that still fail are logged and counted, and the call only errors when every
// snapshot failed.
func (s *portfolioSnapshotService) ComputeAndRecordSnapshots(recordedAt time.Time) (int, error) {
	snapshots, err := s.computeAllSnapshots(recordedAt)
	if err != nil {
		return 0, err
	}

	recorded, failed := s.upsertSnapshots(snapshots)
	if failed > 0 {
		logger.Get().Errorw("portfolio snapshot upserts failed",
			"failed", failed, "recorded", recorded, "recorded_at", recordedAt)
		if recorded == 0 {
			return 0, apperrors.Wrap(apperrors.ErrInternalServer,
				fmt.Errorf("all %d portfolio snapshot upserts failed", failed))
		}
	}
	return recorded, nil
}

// upsertSnapshots writes snapshots with ON CONFLICT (user_id, recorded_at) DO
// UPDATE, returning the number recorded and the number that failed.
func (s *portfolioSnapshotService) upsertSnapshots(snapshots []models.PortfolioSnapshot) (recorded, failed int) {
	onConflict := clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "recorded_at"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"total_net_worth", "cash_balance", "investment_value", "debt_balance", "fixed_deposit_balance",
		}),
	}

	for start := 0; start < len(snapshots); start += snapshotUpsertBatchSize {
		batch := snapshots[start:min(start+snapshotUpsertBatchSize, len(snapshots))]
		err := s.db.Clauses(onConflict).Create(&batch).Error
		if err == nil {
			recorded += len(batch)
			continue
		}
		logger.Get().Warnw("portfolio snapshot batch upsert failed; retrying per user",
			"batch_size", len(batch), "error", err)
		for i := range batch {
			if err := s.db.Clauses(onConflict).Create(&batch[i]).Error; err != nil {
				failed++
				logger.Get().Errorw("portfolio snapshot upsert failed",
					"user_id", batch[i].UserID, "error", err)
				continue
			}
			recorded++
		}
	}
	return recorded, failed
}

// computeAllSnapshots calculates the net worth breakdown of every user with at
// least one active account, ordered by user id.
func (s *portfolioSnapshotService) computeAllSnapshots(recordedAt time.Time) ([]models.PortfolioSnapshot, error) {
	// Balance sums per (user, account type) over active, non-deleted accounts.
	type balanceRow struct {
		UserID string
		Type   models.AccountType
		Total  int64
	}
	var balances []balanceRow
	if err := s.db.Model(&models.Account{}).
		Select("user_id, type, COALESCE(SUM(balance), 0) AS total").
		Where("is_active = ?", true).
		Group("user_id, type").
		Scan(&balances).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	byUser := make(map[string]*models.PortfolioSnapshot)
	snapshotFor := func(userID string) *models.PortfolioSnapshot {
		snap, ok := byUser[userID]
		if !ok {
			snap = &models.PortfolioSnapshot{UserID: userID, RecordedAt: recordedAt}
			byUser[userID] = snap
		}
		return snap
	}
	for _, b := range balances {
		snap := snapshotFor(b.UserID)
		switch b.Type {
		case models.AccountTypeCash:
			snap.CashBalance += b.Total
		case models.AccountTypeFixedDeposit:
			// Fixed deposit balance: active fixed_deposit accounts (plan 020, B8).
			snap.FixedDepositBalance += b.Total
		case models.AccountTypeDebt, models.AccountTypeCreditCard:
			// Liabilities: balances count as debt and are subtracted from net worth.
			snap.DebtBalance += b.Total
		}
	}

	// Investment value: quantity * latest price over holdings in active
	// investment accounts (each holding rounded to the nearest cent).
	type holdingRow struct {
		UserID     string
		SecurityID string
		Quantity   float64
	}
	var holdings []holdingRow
	if err := s.db.Table("investments").
		Select("accounts.user_id, investments.security_id, investments.quantity").
		Joins("JOIN accounts ON accounts.id = investments.account_id").
		Where("accounts.type = ? AND accounts.is_active = ? AND accounts.deleted_at IS NULL AND investments.deleted_at IS NULL",
			models.AccountTypeInvestment, true).
		Scan(&holdings).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	secIDs := make([]string, 0, len(holdings))
	for i := range holdings {
		secIDs = append(secIDs, holdings[i].SecurityID)
	}
	prices, err := getLatestPrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}
	for _, h := range holdings {
		snapshotFor(h.UserID).InvestmentValue += valueCents(h.Quantity, prices[h.SecurityID])
	}

	out := make([]models.PortfolioSnapshot, 0, len(byUser))
	for _, snap := range byUser {
		snap.TotalNetWorth = snap.CashBalance + snap.InvestmentValue + snap.FixedDepositBalance - snap.DebtBalance
		out = append(out, *snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

// computeSnapshot calculates a single user's net worth breakdown. A user with
// no active accounts gets an all-zero snapshot.
func (s *portfolioSnapshotService) computeSnapshot(userID string, recordedAt time.Time) (*models.PortfolioSnapshot, error) {
	all, err := s.computeAllSnapshots(recordedAt)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].UserID == userID {
			return &all[i], nil
		}
	}
	return &models.PortfolioSnapshot{UserID: userID, RecordedAt: recordedAt}, nil
}

// Range caps for grouped snapshots: one bucket per day for up to ~10 years
// (the web "ALL" range is 120 months) and one per hour for up to 93 days.
const (
	maxGroupedDayRange  = 3700 * 24 * time.Hour
	maxGroupedHourRange = 93 * 24 * time.Hour
)

// GetGroupedSnapshots returns snapshots for a user within a date range, keeping
// only the last snapshot per time bucket (e.g. per day or per hour). The result
// is sorted chronologically (oldest first) and is not paginated. The range is
// capped (maxGroupedDayRange / maxGroupedHourRange). On Postgres the bucketing
// runs in SQL (DISTINCT ON the UTC bucket) so only one row per bucket is read.
func (s *portfolioSnapshotService) GetGroupedSnapshots(
	userID string,
	from, to time.Time,
	groupBy string,
) ([]models.PortfolioSnapshot, error) {
	maxRange, pgFormat := maxGroupedDayRange, "YYYY-MM-DD"
	if groupBy == "hour" {
		maxRange, pgFormat = maxGroupedHourRange, "YYYY-MM-DD-HH24"
	}
	if to.Sub(from) > maxRange {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("date range too large for group_by=%s (max %d days)", groupByLabel(groupBy), int(maxRange.Hours()/24)))
	}

	base := s.db.Where("user_id = ? AND recorded_at >= ? AND recorded_at <= ?", userID, from, to)

	var snapshots []models.PortfolioSnapshot
	if s.db.Name() == "postgres" {
		bucket := fmt.Sprintf("to_char(recorded_at AT TIME ZONE 'UTC', '%s')", pgFormat)
		if err := base.
			Select("DISTINCT ON (" + bucket + ") *").
			Order(bucket + " ASC, recorded_at DESC").
			Find(&snapshots).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return snapshots, nil
	}

	if err := base.Order("recorded_at ASC").Find(&snapshots).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return groupSnapshots(snapshots, groupBy), nil
}

// groupByLabel returns the effective bucket name for a group_by value.
func groupByLabel(groupBy string) string {
	if groupBy == "hour" {
		return "hour"
	}
	return "day"
}

// groupSnapshots deduplicates snapshots by keeping the last one per time bucket.
// Since the input must be sorted by recorded_at ASC, iterating and replacing on
// the same bucket key naturally retains the latest snapshot in each bucket.
func groupSnapshots(snapshots []models.PortfolioSnapshot, groupBy string) []models.PortfolioSnapshot {
	if len(snapshots) == 0 {
		return snapshots
	}

	bucketKey := dayBucketKey
	if groupBy == "hour" {
		bucketKey = hourBucketKey
	}

	var result []models.PortfolioSnapshot
	currentKey := ""
	for i := range snapshots {
		key := bucketKey(snapshots[i].RecordedAt)
		if key != currentKey {
			currentKey = key
			result = append(result, snapshots[i])
		} else {
			// Same bucket — replace with the later snapshot.
			result[len(result)-1] = snapshots[i]
		}
	}
	return result
}

func dayBucketKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func hourBucketKey(t time.Time) string {
	return t.UTC().Format("2006-01-02-15")
}

// GetSnapshots returns paginated snapshots for a user within a date range.
func (s *portfolioSnapshotService) GetSnapshots(
	userID string,
	from, to time.Time,
	page pagination.PageRequest,
) (*pagination.PageResponse[models.PortfolioSnapshot], error) {
	page.Defaults()

	var totalItems int64
	base := s.db.Model(&models.PortfolioSnapshot{}).
		Where("user_id = ? AND recorded_at >= ? AND recorded_at <= ?", userID, from, to)
	if err := base.Count(&totalItems).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	var snapshots []models.PortfolioSnapshot
	if err := base.Order("recorded_at DESC").Scopes(pagination.Paginate(page)).Find(&snapshots).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	result := pagination.NewPageResponse(snapshots, page.Page, page.PageSize, totalItems)
	return &result, nil
}
