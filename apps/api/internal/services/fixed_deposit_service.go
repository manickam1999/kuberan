// Package services provides business logic for the Kuberan API.
package services

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/services/fdmath"
)

// FixedDepositServicer defines the contract for scheduled fixed-deposit processing.
type FixedDepositServicer interface {
	// ProcessFixedDeposits runs the nightly scheduled job for a given reference time.
	// It posts periodic interest payouts for all eligible active FDs, advances their
	// NextPayoutDate, and flips the status of matured FDs. Safe to call repeatedly
	// on the same day; idempotency is guaranteed by keying the payout update on the
	// current NextPayoutDate value.
	ProcessFixedDeposits(asOf time.Time) (FDProcessResult, error)
}

// FDProcessResult carries the counts from a single ProcessFixedDeposits run.
type FDProcessResult struct {
	PayoutsPosted int
	FDsMatured    int
}

// fixedDepositService implements FixedDepositServicer.
type fixedDepositService struct {
	db              *gorm.DB
	categoryService CategoryServicer
}

// NewFixedDepositService constructs a FixedDepositServicer.
func NewFixedDepositService(db *gorm.DB, categoryService CategoryServicer) FixedDepositServicer {
	return &fixedDepositService{db: db, categoryService: categoryService}
}

// ProcessFixedDeposits is the main scheduled-processing entry point.
//
// Periodic payouts: for every active periodic FD account where NextPayoutDate <= asOf and
// NextPayoutDate <= MaturityDate, compute PeriodicInterest and post it as an income
// transaction to the linked account. The update is guarded on the exact current
// NextPayoutDate value (WHERE next_payout_date = <current>) so a concurrent or
// same-day re-run that already advanced the date cannot double-post.
//
// Maturity flip: any active FD whose MaturityDate <= asOfDate is set to matured with
// NextPayoutDate cleared. No money movement - realization is manual.
func (s *fixedDepositService) ProcessFixedDeposits(asOf time.Time) (FDProcessResult, error) {
	log := logger.Get()
	result := FDProcessResult{}

	// Normalize asOf to UTC midnight so date comparisons are stable.
	asOfDate := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)

	// --- 1. Periodic payouts ---
	// Load active fixed_deposit accounts that have periodic payout and a due NextPayoutDate.
	// We join on fixed_deposit_details to filter by status/payout_type/next_payout_date.
	var fdAccounts []models.Account
	err := s.db.
		Preload("FixedDeposit").
		Joins("JOIN fixed_deposit_details fdd ON fdd.account_id = accounts.id AND fdd.deleted_at IS NULL").
		Where("accounts.type = ? AND fdd.status = ? AND fdd.payout_type = ? AND fdd.next_payout_date IS NOT NULL AND fdd.next_payout_date <= ?",
			models.AccountTypeFixedDeposit,
			models.FixedDepositStatusActive,
			models.PayoutTypePeriodic,
			asOfDate,
		).
		Find(&fdAccounts).Error
	if err != nil {
		return result, fmt.Errorf("querying periodic FD accounts: %w", err)
	}

	for i := range fdAccounts {
		acct := &fdAccounts[i]
		if acct.FixedDeposit == nil {
			continue
		}
		posted, err := s.processPeriodicPayouts(acct, asOfDate)
		if err != nil {
			log.Warnw("failed to process periodic payouts for FD",
				zap.String("account_id", acct.ID),
				zap.Error(err))
			continue
		}
		result.PayoutsPosted += posted
	}

	// --- 2. Maturity flip ---
	maturityFlipped, err := s.flipMaturedFDs(asOfDate)
	if err != nil {
		return result, fmt.Errorf("flipping matured FDs: %w", err)
	}
	result.FDsMatured = maturityFlipped

	return result, nil
}

// processPeriodicPayouts handles all due payout periods for a single FD account.
// Returns the number of payouts successfully posted.
func (s *fixedDepositService) processPeriodicPayouts(acct *models.Account, asOfDate time.Time) (int, error) {
	log := logger.Get()
	fd := acct.FixedDeposit

	if fd.LinkedAccountID == nil {
		log.Warnw("periodic FD has no linked account, skipping", zap.String("account_id", acct.ID))
		return 0, nil
	}

	if fd.PayoutFrequency == nil {
		log.Warnw("periodic FD has no payout frequency, skipping", zap.String("account_id", acct.ID))
		return 0, nil
	}

	periodsPerYear, ok := fdmath.PayoutFrequencyPeriods(string(*fd.PayoutFrequency))
	if !ok {
		log.Warnw("unrecognized payout frequency",
			zap.String("account_id", acct.ID),
			zap.String("frequency", string(*fd.PayoutFrequency)))
		return 0, nil
	}

	// Ensure the interest income category exists for this user.
	category, err := s.categoryService.EnsureInterestIncomeCategory(acct.UserID)
	if err != nil {
		return 0, fmt.Errorf("ensure interest income category: %w", err)
	}

	posted := 0
	currentNext := fd.NextPayoutDate

	maturityDate := time.Date(fd.MaturityDate.Year(), fd.MaturityDate.Month(), fd.MaturityDate.Day(), 0, 0, 0, 0, time.UTC)

	for currentNext != nil {
		nextDate := time.Date(currentNext.Year(), currentNext.Month(), currentNext.Day(), 0, 0, 0, 0, time.UTC)

		// Stop if this payout date is in the future or past maturity.
		if nextDate.After(asOfDate) || nextDate.After(maturityDate) {
			break
		}

		interestAmount := fdmath.PeriodicInterest(fd.Principal, acct.InterestRate, periodsPerYear)

		// Compute the next payout date after this one.
		advancedDate := advanceByFrequency(nextDate, *fd.PayoutFrequency)

		// Post payout atomically: income transaction + linked account balance credit + NextPayoutDate advance.
		// The WHERE clause on next_payout_date = currentNext is the idempotency guard: if another
		// process already advanced the date, RowsAffected == 0 and we skip without double-posting.
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			// Guard: only proceed if NextPayoutDate still matches the value we read.
			updateResult := tx.Model(&models.FixedDepositDetails{}).
				Where("account_id = ? AND next_payout_date = ? AND status = ?",
					fd.AccountID, currentNext, models.FixedDepositStatusActive).
				Updates(map[string]interface{}{
					"next_payout_date": advancedDate,
				})
			if updateResult.Error != nil {
				return fmt.Errorf("advancing next_payout_date: %w", updateResult.Error)
			}
			if updateResult.RowsAffected == 0 {
				// Another process already advanced this date - skip without error.
				return errFDAlreadyProcessed
			}

			// Post income transaction to linked account.
			txn := &models.Transaction{
				UserID:      acct.UserID,
				AccountID:   *fd.LinkedAccountID,
				CategoryID:  &category.ID,
				Type:        models.TransactionTypeIncome,
				Amount:      interestAmount,
				Description: fmt.Sprintf("Interest payout from %s", acct.Name),
				Date:        nextDate,
			}
			if err := tx.Create(txn).Error; err != nil {
				return fmt.Errorf("creating interest transaction: %w", err)
			}

			// Credit the linked account's balance.
			if err := tx.Model(&models.Account{}).
				Where("id = ?", *fd.LinkedAccountID).
				UpdateColumn("balance", gorm.Expr("balance + ?", interestAmount)).Error; err != nil {
				return fmt.Errorf("crediting linked account balance: %w", err)
			}

			return nil
		})

		if txErr == errFDAlreadyProcessed {
			// Already posted for this period - stop looping for this FD.
			break
		}
		if txErr != nil {
			return posted, fmt.Errorf("posting payout for date %s: %w", nextDate.Format("2006-01-02"), txErr)
		}

		posted++
		currentNext = &advancedDate
	}

	return posted, nil
}

// errFDAlreadyProcessed is a sentinel used to signal that the idempotency guard
// fired (another process already advanced NextPayoutDate). It is not propagated
// to callers.
var errFDAlreadyProcessed = fmt.Errorf("fd already processed")

// flipMaturedFDs sets status=matured and clears NextPayoutDate for every active FD
// whose MaturityDate <= asOfDate. Returns the number of rows updated.
func (s *fixedDepositService) flipMaturedFDs(asOfDate time.Time) (int, error) {
	result := s.db.Model(&models.FixedDepositDetails{}).
		Where("status = ? AND maturity_date <= ?", models.FixedDepositStatusActive, asOfDate).
		Updates(map[string]interface{}{
			"status":           models.FixedDepositStatusMatured,
			"next_payout_date": clause.Expr{SQL: "NULL"},
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// advanceByFrequency returns a new date advanced by one payout interval.
// Monthly -> +1 month, Quarterly -> +3 months, Semiannual -> +6 months, Annual -> +1 year.
func advanceByFrequency(d time.Time, freq models.PayoutFrequency) time.Time {
	switch freq {
	case models.PayoutFrequencyMonthly:
		return d.AddDate(0, 1, 0)
	case models.PayoutFrequencyQuarterly:
		return d.AddDate(0, 3, 0)
	case models.PayoutFrequencySemiannual:
		return d.AddDate(0, 6, 0)
	case models.PayoutFrequencyAnnual:
		return d.AddDate(1, 0, 0)
	default:
		return d.AddDate(0, 1, 0) // safe fallback
	}
}
