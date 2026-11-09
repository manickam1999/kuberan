package services

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
)

// Limits for the balance history endpoints.
const (
	// maxBalanceHistoryDays is the longest range GetBalanceHistory accepts (about
	// ten years of daily rows, like grouped portfolio snapshots).
	maxBalanceHistoryDays = 3700
	// defaultBalanceHistoryDays is the range used when from is omitted.
	defaultBalanceHistoryDays = 90
	// DefaultSparklineDays and MaxSparklineDays bound GetSparklines's days.
	DefaultSparklineDays = 30
	// MaxSparklineDays is the largest accepted sparkline window.
	MaxSparklineDays = 365
	// minSparklineDays is the smallest accepted sparkline window.
	minSparklineDays = 2
	// accountSnapshotMaxAge is how far recordedAt may lag the clock for a
	// pipeline run to write account balance rows. The rows hold the balances as
	// they are now, so a run that stamps a past instant must not write them.
	accountSnapshotMaxAge = 36 * time.Hour
	// accountSnapshotBatch bounds the rows per multi-row insert.
	accountSnapshotBatch = 500
)

// BalancePoint is one day's balance, in the account's own currency (cents).
type BalancePoint struct {
	Day     string `json:"day"` // UTC day, YYYY-MM-DD
	Balance int64  `json:"balance"`
}

// AccountBalanceHistory is the daily balance series of one account.
type AccountBalanceHistory struct {
	AccountID string         `json:"account_id"`
	Currency  string         `json:"currency"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Points    []BalancePoint `json:"points"`
}

// AccountSparkline is the recent daily balances of one account.
type AccountSparkline struct {
	AccountID string         `json:"account_id"`
	Currency  string         `json:"currency"`
	Points    []BalancePoint `json:"points"`
}

// AccountBalanceHistoryServicer reads per-account balance history (plan 023, 5.1).
// The rows are written by the snapshot pipeline (ComputeAndRecordSnapshots).
type AccountBalanceHistoryServicer interface {
	// GetBalanceHistory returns the user's account balance per recorded UTC day
	// in [from, to]. ErrAccountNotFound when the account is not the user's.
	GetBalanceHistory(userID, accountID string, from, to time.Time) (*AccountBalanceHistory, error)
	// GetSparklines returns the last `days` days of balances of each of the
	// user's active accounts.
	GetSparklines(userID string, days int) ([]AccountSparkline, error)
}

type accountBalanceHistoryService struct {
	db *gorm.DB
}

// NewAccountBalanceHistoryService creates the AccountBalanceHistoryServicer.
func NewAccountBalanceHistoryService(db *gorm.DB) AccountBalanceHistoryServicer {
	return &accountBalanceHistoryService{db: db}
}

// GetBalanceHistory implements AccountBalanceHistoryServicer. from defaults to
// 90 days before to, to defaults to today (UTC); the range is capped.
func (s *accountBalanceHistoryService) GetBalanceHistory(userID, accountID string, from, to time.Time) (*AccountBalanceHistory, error) {
	var account models.Account
	if err := s.db.Select("id", "currency").Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAccountNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}

	last := utcDay(to)
	if to.IsZero() {
		last = utcDay(time.Now())
	}
	start := utcDay(from)
	if from.IsZero() {
		start = last.AddDate(0, 0, -defaultBalanceHistoryDays)
	}
	if last.Before(start) {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "to must not be before from")
	}
	if last.Sub(start) > maxBalanceHistoryDays*24*time.Hour {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("date range cannot exceed %d days", maxBalanceHistoryDays))
	}

	var rows []models.AccountBalanceSnapshot
	if err := s.db.Where("account_id = ? AND user_id = ? AND day >= ? AND day <= ?", accountID, userID, start, last).
		Order("day ASC").Find(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &AccountBalanceHistory{
		AccountID: accountID,
		Currency:  normalizeCode(account.Currency),
		From:      start.Format(dayLayout),
		To:        last.Format(dayLayout),
		Points:    toBalancePoints(rows),
	}, nil
}

// GetSparklines implements AccountBalanceHistoryServicer.
func (s *accountBalanceHistoryService) GetSparklines(userID string, days int) ([]AccountSparkline, error) {
	if days == 0 {
		days = DefaultSparklineDays
	}
	if days < minSparklineDays || days > MaxSparklineDays {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput,
			fmt.Sprintf("days must be between %d and %d", minSparklineDays, MaxSparklineDays))
	}

	var accounts []models.Account
	if err := s.db.Select("id", "currency").Where("user_id = ? AND is_active = ?", userID, true).
		Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	out := make([]AccountSparkline, 0, len(accounts))
	if len(accounts) == 0 {
		return out, nil
	}

	start := utcDay(time.Now()).AddDate(0, 0, -(days - 1))
	var rows []models.AccountBalanceSnapshot
	if err := s.db.Where("user_id = ? AND day >= ?", userID, start).
		Order("day ASC").Find(&rows).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	byAccount := make(map[string][]models.AccountBalanceSnapshot, len(accounts))
	for _, r := range rows {
		byAccount[r.AccountID] = append(byAccount[r.AccountID], r)
	}
	for i := range accounts {
		a := &accounts[i]
		out = append(out, AccountSparkline{
			AccountID: a.ID,
			Currency:  normalizeCode(a.Currency),
			Points:    toBalancePoints(byAccount[a.ID]),
		})
	}
	return out, nil
}

// toBalancePoints maps rows to points; the result is never nil.
func toBalancePoints(rows []models.AccountBalanceSnapshot) []BalancePoint {
	points := make([]BalancePoint, 0, len(rows))
	for _, r := range rows {
		points = append(points, BalancePoint{Day: r.Day.UTC().Format(dayLayout), Balance: r.Balance})
	}
	return points
}

// recordAccountBalances writes today's balance row (UTC day of recordedAt) for
// every active account, upserting on (account_id, day), and first backfills the
// history of accounts that have none yet. Investment accounts record their
// market value in the account's currency, the same figure the account list
// shows; every other type records its stored balance.
//
// It does nothing when recordedAt is not within accountSnapshotMaxAge of now:
// the rows hold balances as they are now, so a run stamped with a past instant
// (a replayed or backdated net worth snapshot) must not write them.
func recordAccountBalances(db *gorm.DB, fxs FXServicer, recordedAt time.Time) error {
	if d := time.Since(recordedAt); d > accountSnapshotMaxAge || d < -accountSnapshotMaxAge {
		return nil
	}
	today := utcDay(recordedAt)

	var accounts []models.Account
	if err := db.Select("id", "user_id", "type", "currency", "balance").
		Where("is_active = ?", true).Find(&accounts).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(accounts) == 0 {
		return nil
	}

	// Decide the backfill before today's rows exist, or every account would
	// already have history.
	if err := backfillAccountHistory(db, accounts, today); err != nil {
		// The backfill is an optimisation of the history, never a reason to
		// lose today's rows; it is retried on the next run.
		logger.Get().Errorw("account balance backfill failed", "error", err)
	}

	values, err := investmentAccountValues(db, fxs, accounts, recordedAt)
	if err != nil {
		return err
	}
	rows := make([]models.AccountBalanceSnapshot, 0, len(accounts))
	for i := range accounts {
		a := &accounts[i]
		balance := a.Balance
		if a.Type == models.AccountTypeInvestment {
			v, ok := values[a.ID]
			if !ok {
				continue // holdings without a rate: skip rather than record a wrong value
			}
			balance = v
		}
		rows = append(rows, models.AccountBalanceSnapshot{
			AccountID: a.ID, UserID: a.UserID, Day: today, Balance: balance, Currency: normalizeCode(a.Currency),
		})
	}
	return insertAccountSnapshots(db, rows, clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "balance", "currency"}),
	})
}

// insertAccountSnapshots inserts rows in batches with the given conflict clause.
func insertAccountSnapshots(db *gorm.DB, rows []models.AccountBalanceSnapshot, onConflict clause.OnConflict) error {
	for start := 0; start < len(rows); start += accountSnapshotBatch {
		batch := rows[start:min(start+accountSnapshotBatch, len(rows))]
		if err := db.Clauses(onConflict).Create(&batch).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
	}
	return nil
}

// investmentAccountValues returns the market value of each investment account
// in its own currency (holdings valued at their latest price, converted once
// per line at `at`). An account with no holdings is worth 0; an account with a
// holding that has no FX rate is absent from the result.
func investmentAccountValues(db *gorm.DB, fxs FXServicer, accounts []models.Account, at time.Time) (map[string]int64, error) {
	currencyOf := map[string]string{}
	targets := map[string]struct{}{}
	ids := []string{}
	for i := range accounts {
		a := &accounts[i]
		if a.Type == models.AccountTypeInvestment {
			c := normalizeCode(a.Currency)
			currencyOf[a.ID] = c
			targets[c] = struct{}{}
			ids = append(ids, a.ID)
		}
	}
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		out[id] = 0
	}

	type holding struct {
		AccountID  string
		SecurityID string
		Quantity   float64
	}
	var holdings []holding
	if err := db.Table("investments").Select("account_id, security_id, quantity").
		Where("account_id IN ? AND deleted_at IS NULL", ids).Scan(&holdings).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(holdings) == 0 {
		return out, nil
	}
	secIDs := make([]string, 0, len(holdings))
	for _, h := range holdings {
		secIDs = append(secIDs, h.SecurityID)
	}
	prices, err := latestNativePrices(db, secIDs)
	if err != nil {
		return nil, err
	}
	priceCurrencies := distinctPriceCurrencies(prices)
	valuers := make(map[string]*valuer, len(targets))
	for target := range targets {
		v, vErr := newValuer(fxs, target, at, priceCurrencies)
		if vErr != nil {
			return nil, vErr
		}
		valuers[target] = v
	}
	missing := map[string]bool{}
	for _, h := range holdings {
		p, ok := prices[h.SecurityID]
		if !ok || missing[h.AccountID] {
			continue
		}
		value, _, ok := valuers[currencyOf[h.AccountID]].Convert(valueCents(h.Quantity, p.Price), p.Currency)
		if !ok {
			missing[h.AccountID] = true
			delete(out, h.AccountID)
			continue
		}
		out[h.AccountID] += value
	}
	return out, nil
}

// backfillAccountHistory replays the history of every account that has no
// balance rows yet, writing one row per UTC day from the account's first
// transaction day up to the day before `today`. It runs from the snapshot
// pipeline rather than a migration (SQL can't run this Go replay) and is
// idempotent: an account is only touched while it has no rows, and rows are
// inserted with DO NOTHING.
//
// Replay is exact only for cash, credit card, debt and fixed deposit accounts
// with no `investment` transaction rows (the direction of those is not stored
// on the row; an investment account's balance is market value). Even then the
// replayed total must equal the stored balance, else the account is skipped
// with a warning, so a wrong history is never written.
func backfillAccountHistory(db *gorm.DB, accounts []models.Account, today time.Time) error {
	candidates := make([]models.Account, 0, len(accounts))
	ids := make([]string, 0, len(accounts))
	for i := range accounts {
		a := &accounts[i]
		if a.Type == models.AccountTypeInvestment {
			continue
		}
		candidates = append(candidates, *a)
		ids = append(ids, a.ID)
	}
	if len(candidates) == 0 {
		return nil
	}
	have := map[string]struct{}{}
	for start := 0; start < len(ids); start += accountSnapshotBatch {
		var existing []string
		if err := db.Model(&models.AccountBalanceSnapshot{}).
			Where("account_id IN ?", ids[start:min(start+accountSnapshotBatch, len(ids))]).
			Distinct().Pluck("account_id", &existing).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		for _, id := range existing {
			have[id] = struct{}{}
		}
	}

	for i := range candidates {
		a := &candidates[i]
		if _, ok := have[a.ID]; ok {
			continue
		}
		rows, ok, err := replayAccountHistory(db, a, today)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := insertAccountSnapshots(db, rows, clause.OnConflict{DoNothing: true}); err != nil {
			return err
		}
	}
	return nil
}

// replayAccountHistory rebuilds the daily balances of one account from its
// transactions. ok is false when replay is not exact for this account.
func replayAccountHistory(db *gorm.DB, a *models.Account, today time.Time) (rows []models.AccountBalanceSnapshot, ok bool, err error) {
	type txRow struct {
		AccountID   string
		ToAccountID *string
		Type        models.TransactionType
		Amount      int64
		ToAmount    *int64
		Date        time.Time
	}
	var txs []txRow
	if err := db.Table("transactions").
		Select("account_id, to_account_id, type, amount, to_amount, date").
		Where("user_id = ? AND deleted_at IS NULL AND (account_id = ? OR to_account_id = ?)", a.UserID, a.ID, a.ID).
		Scan(&txs).Error; err != nil {
		return nil, false, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if len(txs) == 0 {
		return nil, false, nil
	}

	deltas := map[time.Time]int64{}
	var total int64
	for _, t := range txs {
		var delta int64
		switch {
		case t.AccountID == a.ID && t.Type == models.TransactionTypeTransfer:
			delta, _ = balanceDelta(a.Type, models.TransactionTypeExpense, t.Amount)
		case t.AccountID == a.ID && t.Type == models.TransactionTypeInvestment:
			return nil, false, nil // direction not stored: not replayable
		case t.AccountID == a.ID:
			d, known := balanceDelta(a.Type, t.Type, t.Amount)
			if !known {
				return nil, false, nil
			}
			delta = d
		case t.ToAccountID != nil && *t.ToAccountID == a.ID && t.Type == models.TransactionTypeTransfer:
			amount := t.Amount
			if t.ToAmount != nil {
				amount = *t.ToAmount
			}
			delta, _ = balanceDelta(a.Type, models.TransactionTypeIncome, amount)
		default:
			continue
		}
		deltas[utcDay(t.Date)] += delta
		total += delta
	}
	if total != a.Balance {
		logger.Get().Warnw("account balance replay does not match the stored balance; history not backfilled",
			"account_id", a.ID, "replayed", total, "stored", a.Balance)
		return nil, false, nil
	}

	days := make([]time.Time, 0, len(deltas))
	for d := range deltas {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	currency := normalizeCode(a.Currency)
	var running int64
	next := 0
	for d := days[0]; d.Before(today); d = d.AddDate(0, 0, 1) {
		for next < len(days) && !days[next].After(d) {
			running += deltas[days[next]]
			next++
		}
		rows = append(rows, models.AccountBalanceSnapshot{
			AccountID: a.ID, UserID: a.UserID, Day: d, Balance: running, Currency: currency,
		})
	}
	return rows, true, nil
}
