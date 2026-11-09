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
	db        *gorm.DB
	fxService FXServicer
}

// NewPortfolioSnapshotService creates a new PortfolioSnapshotServicer. fxs
// converts holdings into each user's base currency; production wiring always
// passes the FX service. A nil fxs is allowed in tests only: then only
// same-currency holdings are valued.
func NewPortfolioSnapshotService(db *gorm.DB, fxs FXServicer) PortfolioSnapshotServicer {
	return &portfolioSnapshotService{db: db, fxService: fxs}
}

// snapshotUpsertBatchSize bounds the rows per multi-row snapshot upsert.
const snapshotUpsertBatchSize = 500

// ComputeAndRecordSnapshots computes and stores a net worth snapshot for all
// active users and returns how many were recorded.
//
// The computation is set-based: one GROUP BY (user_id, type, currency) query for account
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

	// Per-account daily balances (plan 023, 5.1). A failure here is logged and
	// never fails the net worth snapshots; the next run (30 minutes later)
	// writes the same day's rows.
	if err := recordAccountBalances(s.db, s.fxService, recordedAt); err != nil {
		logger.Get().Errorw("account balance snapshots failed", "recorded_at", recordedAt, "error", err)
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
			"currency", "fx_missing",
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
		UserID   string
		Type     models.AccountType
		Currency string
		Total    int64
	}
	var balances []balanceRow
	if err := s.db.Model(&models.Account{}).
		Select("user_id, type, UPPER(TRIM(currency)) AS currency, COALESCE(SUM(balance), 0) AS total").
		Where("is_active = ?", true).
		Group("user_id, type, UPPER(TRIM(currency))").
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
		snapshotFor(b.UserID)
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
		snapshotFor(holdings[i].UserID)
		secIDs = append(secIDs, holdings[i].SecurityID)
	}
	prices, err := latestNativePrices(s.db, secIDs)
	if err != nil {
		return nil, err
	}

	// Each snapshot is reported in its user's base currency; holdings are
	// converted at recordedAt with one Rates call per distinct base currency.
	userIDs := make([]string, 0, len(byUser))
	for id := range byUser {
		userIDs = append(userIDs, id)
	}
	baseOf, err := s.baseCurrencies(userIDs)
	if err != nil {
		return nil, err
	}
	currencySet := map[string]struct{}{}
	for _, c := range distinctPriceCurrencies(prices) {
		currencySet[c] = struct{}{}
	}
	for _, b := range balances {
		currencySet[b.Currency] = struct{}{}
	}
	priceCurrencies := make([]string, 0, len(currencySet))
	for c := range currencySet {
		priceCurrencies = append(priceCurrencies, c)
	}
	sort.Strings(priceCurrencies)
	valuers := map[string]*valuer{}
	for _, id := range userIDs {
		base := baseOf[id]
		if _, ok := valuers[base]; ok {
			continue
		}
		v, vErr := newValuer(s.fxService, base, recordedAt, priceCurrencies)
		if vErr != nil {
			return nil, vErr
		}
		valuers[base] = v
	}
	for _, id := range userIDs {
		byUser[id].Currency = baseOf[id]
	}
	// Account balances are converted per (type, currency) group, once per
	// group, at recordedAt. A group without a rate is left out of the totals
	// (never counted 1:1) and flags the snapshot.
	for _, b := range balances {
		snap := byUser[b.UserID]
		value, _, ok := valuers[snap.Currency].Convert(b.Total, b.Currency)
		if !ok {
			snap.FXMissing = true
			logger.Get().Warnw("snapshot balance group skipped: no FX rate",
				"user_id", b.UserID, "base_currency", snap.Currency, "account_currency", b.Currency,
				"account_type", string(b.Type))
			continue
		}
		switch b.Type {
		case models.AccountTypeCash:
			snap.CashBalance += value
		case models.AccountTypeFixedDeposit:
			// Fixed deposit balance: active fixed_deposit accounts (plan 020, B8).
			snap.FixedDepositBalance += value
		case models.AccountTypeDebt, models.AccountTypeCreditCard:
			// Liabilities: balances count as debt and are subtracted from net worth.
			snap.DebtBalance += value
		}
	}
	for _, h := range holdings {
		snap := snapshotFor(h.UserID)
		p := prices[h.SecurityID]
		value, _, ok := valuers[snap.Currency].Convert(valueCents(h.Quantity, p.Price), p.Currency)
		if !ok {
			snap.FXMissing = true
			logger.Get().Warnw("snapshot holding skipped: no FX rate",
				"user_id", h.UserID, "base_currency", snap.Currency, "price_currency", p.Currency,
				"security_id", h.SecurityID)
			continue
		}
		snap.InvestmentValue += value
	}

	out := make([]models.PortfolioSnapshot, 0, len(byUser))
	for _, snap := range byUser {
		snap.TotalNetWorth = snap.CashBalance + snap.InvestmentValue + snap.FixedDepositBalance - snap.DebtBalance
		out = append(out, *snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

// baseCurrencies returns each user's base currency (upper-case), defaulting to
// the column default for users that cannot be read.
func (s *portfolioSnapshotService) baseCurrencies(userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	for _, id := range userIDs {
		out[id] = defaultBaseCurrency
	}
	const chunk = 500
	for start := 0; start < len(userIDs); start += chunk {
		ids := userIDs[start:min(start+chunk, len(userIDs))]
		var rows []struct {
			ID           string
			BaseCurrency string
		}
		if err := s.db.Table("users").Select("id, base_currency").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
			return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for _, r := range rows {
			if c := normalizeCode(r.BaseCurrency); c != "" {
				out[r.ID] = c
			}
		}
	}
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
