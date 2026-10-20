package services

// Tests for the fixed-deposit surface of accountService (plan 020, B10).
//
// Coverage:
//   - CreateFixedDepositAccount: happy path, source debited/FD credited, transfer tx,
//     maturity_value computed, next_payout_date set for periodic/nil for cumulative,
//     rejections (insufficient balance, non-cash source, bad dates, missing freq).
//   - CloseFixedDeposit: cumulative matured, cumulative premature with accrued interest,
//     periodic (principal only), interest_override, penalty floors at 0, already-closed,
//     destination defaults to linked account.
//   - UpdateAccount FD mutable fields: rate/maturity/compounding update + MV recomputed;
//     principal/start_date immutable.

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func fdDate(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func ptrFreq(f models.PayoutFrequency) *models.PayoutFrequency { return &f }
func ptrStr(s string) *string                                  { return &s }
func ptrInt64(n int64) *int64                                  { return &n }
func ptrComp(c models.Compounding) *models.Compounding         { return &c }

// ---------------------------------------------------------------------------
// TestCreateFixedDepositAccount
// ---------------------------------------------------------------------------

func TestCreateFixedDepositAccount(t *testing.T) {
	t.Run("funds_from_source_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000) // $5,000.00

		start := fdDate(2025, 1, 1)
		maturity := fdDate(2026, 1, 1)

		fdAcct, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "My FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       200000,
			InterestRate:    5.0,
			StartDate:       start,
			MaturityDate:    maturity,
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertNoError(t, err)

		// FD balance should equal principal
		if fdAcct.Balance != 200000 {
			t.Errorf("expected FD balance 200000, got %d", fdAcct.Balance)
		}
		if fdAcct.Type != models.AccountTypeFixedDeposit {
			t.Errorf("expected type fixed_deposit, got %s", fdAcct.Type)
		}

		// Source account should be debited
		var updatedSource models.Account
		db.First(&updatedSource, "id = ?", sourceAcct.ID)
		if updatedSource.Balance != 300000 {
			t.Errorf("expected source balance 300000 after transfer, got %d", updatedSource.Balance)
		}

		// A transfer transaction should be recorded on the source
		var txns []models.Transaction
		db.Where("account_id = ? AND type = ?", sourceAcct.ID, models.TransactionTypeTransfer).Find(&txns)
		if len(txns) != 1 {
			t.Fatalf("expected 1 transfer transaction on source, got %d", len(txns))
		}
		if txns[0].Amount != 200000 {
			t.Errorf("expected transfer amount 200000, got %d", txns[0].Amount)
		}
		if txns[0].ToAccountID == nil || *txns[0].ToAccountID != fdAcct.ID {
			t.Errorf("expected to_account_id = %s, got %v", fdAcct.ID, txns[0].ToAccountID)
		}
	})

	t.Run("maturity_value_computed_and_stored", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		// P=100000, r=5%, annual compounding, 365 days (non-leap 2023) -> MV = 105000 exactly
		start := fdDate(2023, 1, 1)
		maturity := fdDate(2024, 1, 1)

		fdAcct, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "Annual FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       start,
			MaturityDate:    maturity,
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertNoError(t, err)

		if fdAcct.FixedDeposit == nil {
			t.Fatal("expected FixedDeposit details to be attached")
		}
		if fdAcct.FixedDeposit.MaturityValue != 105000 {
			t.Errorf("expected maturity_value 105000, got %d", fdAcct.FixedDeposit.MaturityValue)
		}
	})

	t.Run("next_payout_date_set_for_periodic", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)
		linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		start := fdDate(2025, 1, 15)
		maturity := fdDate(2026, 1, 15)

		fdAcct, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "Periodic FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       start,
			MaturityDate:    maturity,
			PayoutType:      models.PayoutTypePeriodic,
			PayoutFrequency: ptrFreq(models.PayoutFrequencyMonthly),
			Compounding:     models.CompoundingMonthly,
			LinkedAccountID: ptrStr(linkedAcct.ID),
		})
		testutil.AssertNoError(t, err)

		if fdAcct.FixedDeposit == nil {
			t.Fatal("expected FixedDeposit details")
		}
		// Monthly from Jan 15 -> next payout is Feb 15
		expectedNext := fdDate(2025, 2, 15)
		if fdAcct.FixedDeposit.NextPayoutDate == nil {
			t.Fatal("expected next_payout_date to be set for periodic FD")
		}
		if !fdAcct.FixedDeposit.NextPayoutDate.Equal(expectedNext) {
			t.Errorf("expected next_payout_date %s, got %v",
				expectedNext.Format("2006-01-02"), fdAcct.FixedDeposit.NextPayoutDate)
		}
	})

	t.Run("next_payout_date_nil_for_cumulative", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		fdAcct, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "Cumulative FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertNoError(t, err)

		if fdAcct.FixedDeposit == nil {
			t.Fatal("expected FixedDeposit details")
		}
		if fdAcct.FixedDeposit.NextPayoutDate != nil {
			t.Errorf("expected next_payout_date nil for cumulative FD, got %v", fdAcct.FixedDeposit.NextPayoutDate)
		}
	})

	t.Run("rejects_insufficient_source_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000) // only $100

		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "Too Big FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       200000, // $2,000 - more than available
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertAppError(t, err, "SOURCE_ACCOUNT_INSUFFICIENT")
	})

	t.Run("rejects_non_cash_source", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		investAcct := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "FD",
			Currency:        "USD",
			SourceAccountID: investAcct.ID,
			Principal:       10000,
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertAppError(t, err, "INVALID_ACCOUNT_TYPE")
	})

	t.Run("rejects_maturity_before_start", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       10000,
			InterestRate:    5.0,
			StartDate:       fdDate(2026, 1, 1),
			MaturityDate:    fdDate(2025, 1, 1), // before start
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("rejects_zero_principal", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       0,
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("rejects_periodic_without_frequency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		_, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypePeriodic,
			PayoutFrequency: nil, // missing!
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("fd_status_active_on_create", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		sourceAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500000)

		fdAcct, err := svc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
			Name:            "FD",
			Currency:        "USD",
			SourceAccountID: sourceAcct.ID,
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       fdDate(2025, 1, 1),
			MaturityDate:    fdDate(2026, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Compounding:     models.CompoundingAnnual,
		})
		testutil.AssertNoError(t, err)

		if fdAcct.FixedDeposit == nil {
			t.Fatal("expected FixedDeposit details")
		}
		if fdAcct.FixedDeposit.Status != models.FixedDepositStatusActive {
			t.Errorf("expected status active, got %s", fdAcct.FixedDeposit.Status)
		}
		if !fdAcct.IsActive {
			t.Error("expected FD account to be active")
		}
	})
}

// ---------------------------------------------------------------------------
// TestCloseFixedDeposit
// ---------------------------------------------------------------------------

func TestCloseFixedDeposit(t *testing.T) {
	t.Run("cumulative_matured_principal_and_interest_to_destination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured,
		})
		// Set maturity_value to 105000 (annual 5% on $1000 for 365 days)
		db.Model(fd).Update("maturity_value", 105000)

		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
		})
		testutil.AssertNoError(t, err)

		// FD balance should be 0 and inactive
		var updatedFD models.Account
		db.First(&updatedFD, "id = ?", fdAcct.ID)
		if updatedFD.Balance != 0 {
			t.Errorf("expected FD balance 0 after close, got %d", updatedFD.Balance)
		}
		if updatedFD.IsActive {
			t.Error("expected FD to be inactive after close")
		}

		// Destination should have principal + interest = 105000
		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		if updatedDest.Balance != 105000 {
			t.Errorf("expected dest balance 105000 (principal+interest), got %d", updatedDest.Balance)
		}

		// FD details status should be closed
		var updatedDetails models.FixedDepositDetails
		db.First(&updatedDetails, "account_id = ?", fdAcct.ID)
		if updatedDetails.Status != models.FixedDepositStatusClosed {
			t.Errorf("expected FD status closed, got %s", updatedDetails.Status)
		}
	})

	t.Run("cumulative_matured_interest_transaction_categorized", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured,
		})
		db.Model(fd).Update("maturity_value", 105000)

		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
		})
		testutil.AssertNoError(t, err)

		// Should be an income transaction on the destination for the interest
		var incomeTxns []models.Transaction
		db.Where("account_id = ? AND type = ?", destAcct.ID, models.TransactionTypeIncome).Find(&incomeTxns)
		if len(incomeTxns) != 1 {
			t.Fatalf("expected 1 interest income transaction, got %d", len(incomeTxns))
		}
		if incomeTxns[0].Amount != 5000 {
			t.Errorf("expected interest amount 5000, got %d", incomeTxns[0].Amount)
		}
		// Should have a category set (Interest Income)
		if incomeTxns[0].CategoryID == nil {
			t.Error("expected interest transaction to have a category ID (Interest Income)")
		}
	})

	t.Run("cumulative_premature_interest_override", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2027, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		interestOverride := int64(2500)
		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
			InterestOverride:     &interestOverride,
		})
		testutil.AssertNoError(t, err)

		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		// principal + override interest = 100000 + 2500 = 102500
		if updatedDest.Balance != 102500 {
			t.Errorf("expected dest balance 102500, got %d", updatedDest.Balance)
		}

		var updatedFD models.Account
		db.First(&updatedFD, "id = ?", fdAcct.ID)
		if updatedFD.IsActive {
			t.Error("expected FD to be inactive after premature close")
		}
	})

	t.Run("periodic_close_returns_principal_only", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypePeriodic,
			Status:       models.FixedDepositStatusActive,
		})

		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
		})
		testutil.AssertNoError(t, err)

		// Periodic: interest already handled via payouts; close returns principal only
		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		if updatedDest.Balance != 100000 {
			t.Errorf("expected periodic FD close to return 100000 (principal only), got %d", updatedDest.Balance)
		}

		// No income transaction should be created on close
		var incomeTxns []models.Transaction
		db.Where("account_id = ? AND type = ?", destAcct.ID, models.TransactionTypeIncome).Find(&incomeTxns)
		if len(incomeTxns) != 0 {
			t.Errorf("expected no interest income transaction for periodic FD close, got %d", len(incomeTxns))
		}
	})

	t.Run("interest_override_used", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured,
		})
		db.Model(fd).Update("maturity_value", 105000)

		// Override the computed interest (5000) to 3000
		override := int64(3000)
		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
			InterestOverride:     &override,
		})
		testutil.AssertNoError(t, err)

		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		// principal + override interest = 100000 + 3000 = 103000
		if updatedDest.Balance != 103000 {
			t.Errorf("expected dest balance 103000, got %d", updatedDest.Balance)
		}
	})

	t.Run("penalty_reduces_net_interest", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured,
		})
		db.Model(fd).Update("maturity_value", 105000)

		// Penalty 2000 -> net interest = 5000 - 2000 = 3000
		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
			Penalty:              2000,
		})
		testutil.AssertNoError(t, err)

		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		// principal + net interest = 100000 + 3000 = 103000
		if updatedDest.Balance != 103000 {
			t.Errorf("expected dest balance 103000 (penalty applied), got %d", updatedDest.Balance)
		}
	})

	t.Run("penalty_floors_net_interest_at_zero", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured,
		})
		db.Model(fd).Update("maturity_value", 105000)

		// Penalty 10000 exceeds gross interest 5000 -> net clamped to 0
		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
			Penalty:              10000,
		})
		testutil.AssertNoError(t, err)

		var updatedDest models.Account
		db.First(&updatedDest, "id = ?", destAcct.ID)
		// Only principal, no interest (clamped to 0)
		if updatedDest.Balance != 100000 {
			t.Errorf("expected dest balance 100000 (interest floored to 0), got %d", updatedDest.Balance)
		}

		// No income transaction when net interest == 0
		var incomeTxns []models.Transaction
		db.Where("account_id = ? AND type = ?", destAcct.ID, models.TransactionTypeIncome).Find(&incomeTxns)
		if len(incomeTxns) != 0 {
			t.Errorf("expected no income transaction when net interest is 0, got %d", len(incomeTxns))
		}
	})

	t.Run("rejects_already_closed", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		destAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusClosed,
		})
		// Mark account as closed (inactive) so GetAccountByID (which filters is_active=true) still finds it
		// Note: CloseFixedDeposit loads with is_active excluded, so we need the direct query to work.
		// The service loads with WHERE id = ? AND user_id = ? (no is_active filter) for CloseFixedDeposit.
		_ = fdAcct

		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: ptrStr(destAcct.ID),
		})
		testutil.AssertAppError(t, err, "FIXED_DEPOSIT_ALREADY_CLOSED")
	})

	t.Run("destination_defaults_to_linked_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db, NewCategoryService(db))
		user := testutil.CreateTestUser(t, db)
		linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:       100000,
			InterestRate:    5.0,
			StartDate:       fdDate(2023, 1, 1),
			MaturityDate:    fdDate(2024, 1, 1),
			PayoutType:      models.PayoutTypeCumulative,
			Status:          models.FixedDepositStatusMatured,
			LinkedAccountID: ptrStr(linkedAcct.ID),
		})
		db.Model(fd).Update("maturity_value", 105000)

		// No destination specified; should use linked account
		_, err := svc.CloseFixedDeposit(user.ID, fdAcct.ID, CloseFixedDepositParams{
			DestinationAccountID: nil,
		})
		testutil.AssertNoError(t, err)

		var updatedLinked models.Account
		db.First(&updatedLinked, "id = ?", linkedAcct.ID)
		if updatedLinked.Balance != 105000 {
			t.Errorf("expected linked account balance 105000, got %d", updatedLinked.Balance)
		}
	})
}

// ---------------------------------------------------------------------------
// TestUpdateAccount_FixedDepositFields
// ---------------------------------------------------------------------------

func TestUpdateAccount_FixedDepositFields(t *testing.T) {
	t.Run("updates_interest_rate_and_recomputes_maturity_value", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})
		db.Model(fd).Update("compounding", models.CompoundingAnnual)
		db.Model(fd).Update("maturity_value", 105000)

		// Update rate to 10%; annual compounding for 365 days -> MV = 110000
		newRate := 10.0
		updated, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			InterestRate: &newRate,
		})
		testutil.AssertNoError(t, err)

		if updated.InterestRate != 10.0 {
			t.Errorf("expected interest_rate 10.0, got %f", updated.InterestRate)
		}

		var updatedDetails models.FixedDepositDetails
		db.First(&updatedDetails, "account_id = ?", fdAcct.ID)
		// P=100000, r=10%, annual, 365 days -> MV = 110000
		if updatedDetails.MaturityValue != 110000 {
			t.Errorf("expected recomputed maturity_value 110000, got %d", updatedDetails.MaturityValue)
		}
	})

	t.Run("updates_maturity_date_and_recomputes_maturity_value", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2021, 1, 1),
			MaturityDate: fdDate(2022, 1, 1), // 1 year (365 days, no leap)
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})
		db.Model(fd).Update("compounding", models.CompoundingAnnual)
		db.Model(fd).Update("maturity_value", 105000)

		// Extend to 2 years (2021-01-01 -> 2023-01-01 = 730 days, no leap year).
		newMaturity := fdDate(2023, 1, 1)
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			MaturityDate: &newMaturity,
		})
		testutil.AssertNoError(t, err)

		var updatedDetails models.FixedDepositDetails
		db.First(&updatedDetails, "account_id = ?", fdAcct.ID)
		// 2 years, 5% annual: 100000 * (1.05)^2 = 110250
		if updatedDetails.MaturityValue != 110250 {
			t.Errorf("expected recomputed maturity_value 110250 (2yr 5%% annual), got %d", updatedDetails.MaturityValue)
		}
	})

	t.Run("updates_compounding_and_recomputes_maturity_value", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2023, 1, 1),
			MaturityDate: fdDate(2024, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})
		db.Model(fd).Update("compounding", models.CompoundingAnnual)
		db.Model(fd).Update("maturity_value", 105000)

		// Switch to monthly compounding: 100000*(1+0.05/12)^12 = 105116
		newComp := models.CompoundingMonthly
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			Compounding: &newComp,
		})
		testutil.AssertNoError(t, err)

		var updatedDetails models.FixedDepositDetails
		db.First(&updatedDetails, "account_id = ?", fdAcct.ID)
		if updatedDetails.MaturityValue != 105116 {
			t.Errorf("expected maturity_value 105116 (monthly compounding), got %d", updatedDetails.MaturityValue)
		}
	})

	t.Run("updates_payout_type_and_frequency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		periodic := models.PayoutTypePeriodic
		quarterly := models.PayoutFrequencyQuarterly
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			PayoutType:      &periodic,
			PayoutFrequency: &quarterly,
		})
		testutil.AssertNoError(t, err)

		var updatedDetails models.FixedDepositDetails
		db.First(&updatedDetails, "account_id = ?", fdAcct.ID)
		if updatedDetails.PayoutType != models.PayoutTypePeriodic {
			t.Errorf("expected payout_type periodic, got %s", updatedDetails.PayoutType)
		}
		if updatedDetails.PayoutFrequency == nil || *updatedDetails.PayoutFrequency != models.PayoutFrequencyQuarterly {
			t.Errorf("expected payout_frequency quarterly, got %v", updatedDetails.PayoutFrequency)
		}
	})

	t.Run("principal_unchanged_via_update", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    200000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		name := "Renamed FD"
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			Name: &name,
		})
		testutil.AssertNoError(t, err)

		var details models.FixedDepositDetails
		db.First(&details, "account_id = ?", fdAcct.ID)
		if details.Principal != 200000 {
			t.Errorf("expected principal 200000 (immutable), got %d", details.Principal)
		}
	})

	t.Run("start_date_unchanged_via_update", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		originalStart := fdDate(2025, 1, 1)
		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    originalStart,
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		name := "Updated Name"
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			Name: &name,
		})
		testutil.AssertNoError(t, err)

		var details models.FixedDepositDetails
		db.First(&details, "account_id = ?", fdAcct.ID)
		if !details.StartDate.Equal(originalStart) {
			t.Errorf("expected start_date %v (immutable), got %v", originalStart, details.StartDate)
		}
	})

	t.Run("updates_linked_account_id", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		newLinked := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
		_, err := svc.UpdateAccount(user.ID, fdAcct.ID, AccountUpdateFields{
			LinkedAccountID: ptrStr(newLinked.ID),
		})
		testutil.AssertNoError(t, err)

		var details models.FixedDepositDetails
		db.First(&details, "account_id = ?", fdAcct.ID)
		if details.LinkedAccountID == nil || *details.LinkedAccountID != newLinked.ID {
			t.Errorf("expected linked_account_id = %s, got %v", newLinked.ID, details.LinkedAccountID)
		}
	})
}
