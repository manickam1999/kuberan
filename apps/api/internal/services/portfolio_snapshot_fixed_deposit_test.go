package services

// Tests for fixed-deposit contribution to portfolio snapshots (plan 020, B8 + B10).
//
// Coverage:
//   - Active FD account balance included in fixed_deposit_balance and total_net_worth.
//   - Closed (is_active=false) FD account excluded from snapshot.
//   - Net worth formula: cash + investment + fixed_deposit - debt.
//   - FixedDepositBalance field populated correctly.

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestComputeSnapshot_FixedDeposit(t *testing.T) {
	t.Run("active_fd_included_in_fixed_deposit_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		// Need at least one active account for the user to appear in snapshot
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		// Create an active FD with balance = principal
		testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000, // $1000.00
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		recordedAt := time.Now().Truncate(time.Second)
		count, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)
		if count != 1 {
			t.Fatalf("expected 1 snapshot, got %d", count)
		}

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.FixedDepositBalance != 100000 {
			t.Errorf("expected fixed_deposit_balance 100000, got %d", snap.FixedDepositBalance)
		}
	})

	t.Run("closed_fd_excluded_from_snapshot", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		fdAcct, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2024, 1, 1),
			MaturityDate: fdDate(2025, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusClosed,
		})
		// Mark account inactive (closed FD)
		db.Model(fdAcct).Update("is_active", false)

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.FixedDepositBalance != 0 {
			t.Errorf("expected closed FD excluded from fixed_deposit_balance, got %d", snap.FixedDepositBalance)
		}
	})

	t.Run("net_worth_includes_fixed_deposit", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)

		// Cash: $1500.00
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000) // $1000
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 50000)  // $500

		// Fixed deposit: $2000.00
		testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    200000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		// Debt: $500.00
		testutil.CreateTestDebtAccount(t, db, user.ID, 50000)

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.CashBalance != 150000 {
			t.Errorf("expected cash_balance 150000, got %d", snap.CashBalance)
		}
		if snap.FixedDepositBalance != 200000 {
			t.Errorf("expected fixed_deposit_balance 200000, got %d", snap.FixedDepositBalance)
		}
		if snap.DebtBalance != 50000 {
			t.Errorf("expected debt_balance 50000, got %d", snap.DebtBalance)
		}
		// net_worth = cash + investment + fixed_deposit - debt = 150000 + 0 + 200000 - 50000 = 300000
		expectedNetWorth := int64(150000 + 0 + 200000 - 50000)
		if snap.TotalNetWorth != expectedNetWorth {
			t.Errorf("expected total_net_worth %d, got %d", expectedNetWorth, snap.TotalNetWorth)
		}
	})

	t.Run("multiple_fd_accounts_summed", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		// Two active FDs
		testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2025, 1, 1),
			MaturityDate: fdDate(2026, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})
		testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    250000,
			InterestRate: 4.5,
			StartDate:    fdDate(2025, 3, 1),
			MaturityDate: fdDate(2027, 3, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusActive,
		})

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.FixedDepositBalance != 350000 {
			t.Errorf("expected fixed_deposit_balance 350000 (sum of two FDs), got %d", snap.FixedDepositBalance)
		}
	})

	t.Run("matured_but_still_active_fd_included", func(t *testing.T) {
		// A FD with status=matured is still is_active=true until the user closes it.
		// It should still count in the fixed_deposit_balance.
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewPortfolioSnapshotService(db)

		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

		testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
			Principal:    100000,
			InterestRate: 5.0,
			StartDate:    fdDate(2024, 1, 1),
			MaturityDate: fdDate(2025, 1, 1),
			PayoutType:   models.PayoutTypeCumulative,
			Status:       models.FixedDepositStatusMatured, // matured but still active
		})
		// Account is still is_active = true (not yet closed by user)

		recordedAt := time.Now().Truncate(time.Second)
		_, err := svc.ComputeAndRecordSnapshots(recordedAt)
		testutil.AssertNoError(t, err)

		var snap models.PortfolioSnapshot
		db.Where("user_id = ?", user.ID).First(&snap)

		if snap.FixedDepositBalance != 100000 {
			t.Errorf("expected matured-but-active FD included in fixed_deposit_balance 100000, got %d", snap.FixedDepositBalance)
		}
	})
}
