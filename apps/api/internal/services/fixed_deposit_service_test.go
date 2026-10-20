package services_test

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/testutil"
)

// ptr helpers for test fixtures.
func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }

func TestProcessFixedDeposits_SinglePeriodicPayout(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	start := date(2025, 1, 1)
	maturity := date(2026, 6, 1)
	nextPayout := date(2025, 2, 1) // due Feb 1

	freq := models.PayoutFrequencyMonthly
	_, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       120000, // $1200.00
		InterestRate:    12.0,   // 12% annual -> $12/month
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypePeriodic,
		PayoutFrequency: &freq,
		LinkedAccountID: strPtr(linkedAcct.ID),
		NextPayoutDate:  timePtr(nextPayout),
	})

	// Run as of Feb 1, 2025.
	asOf := date(2025, 2, 1)
	result, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}

	if result.PayoutsPosted != 1 {
		t.Errorf("expected 1 payout posted, got %d", result.PayoutsPosted)
	}
	if result.FDsMatured != 0 {
		t.Errorf("expected 0 FDs matured, got %d", result.FDsMatured)
	}

	// Verify the linked account balance was credited.
	var updatedLinked models.Account
	db.First(&updatedLinked, "id = ?", linkedAcct.ID)
	expectedInterest := int64(1200) // 120000 * 0.12 / 12 = 1200 cents = $12.00
	if updatedLinked.Balance != expectedInterest {
		t.Errorf("linked account balance: want %d, got %d", expectedInterest, updatedLinked.Balance)
	}

	// Verify NextPayoutDate advanced to March 1.
	var updatedFD models.FixedDepositDetails
	db.First(&updatedFD, "account_id = ?", fd.AccountID)
	expectedNext := date(2025, 3, 1)
	if updatedFD.NextPayoutDate == nil || !updatedFD.NextPayoutDate.Equal(expectedNext) {
		t.Errorf("NextPayoutDate: want %s, got %v", expectedNext.Format("2006-01-02"), updatedFD.NextPayoutDate)
	}

	// Verify an income transaction was created.
	var txns []models.Transaction
	db.Where("account_id = ? AND type = ?", linkedAcct.ID, models.TransactionTypeIncome).Find(&txns)
	if len(txns) != 1 {
		t.Errorf("expected 1 income transaction, got %d", len(txns))
	} else if txns[0].Amount != expectedInterest {
		t.Errorf("transaction amount: want %d, got %d", expectedInterest, txns[0].Amount)
	}
}

func TestProcessFixedDeposits_MultipleElapsedPeriods_CorrectCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	start := date(2025, 1, 1)
	maturity := date(2026, 12, 1)
	nextPayout := date(2025, 2, 1)

	freq := models.PayoutFrequencyMonthly
	testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       120000,
		InterestRate:    12.0,
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypePeriodic,
		PayoutFrequency: &freq,
		LinkedAccountID: strPtr(linkedAcct.ID),
		NextPayoutDate:  timePtr(nextPayout),
	})

	// Run as of Apr 30 - should post Feb, Mar, Apr (Apr30 >= Apr1 so Apr posts; May1 > Apr30 so stops).
	asOf := date(2025, 4, 30)
	result, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}

	// Feb1, Mar1, Apr1 all <= Apr30. May1 > Apr30 -> stop. 3 payouts.
	if result.PayoutsPosted != 3 {
		t.Errorf("expected 3 payouts posted, got %d", result.PayoutsPosted)
	}

	var txns []models.Transaction
	db.Where("account_id = ? AND type = ?", linkedAcct.ID, models.TransactionTypeIncome).Find(&txns)
	if len(txns) != 3 {
		t.Errorf("expected 3 income transactions, got %d", len(txns))
	}
}

func TestProcessFixedDeposits_Idempotent_SameDayRerun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	start := date(2025, 1, 1)
	maturity := date(2026, 6, 1)
	nextPayout := date(2025, 2, 1)

	freq := models.PayoutFrequencyMonthly
	testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       120000,
		InterestRate:    12.0,
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypePeriodic,
		PayoutFrequency: &freq,
		LinkedAccountID: strPtr(linkedAcct.ID),
		NextPayoutDate:  timePtr(nextPayout),
	})

	asOf := date(2025, 2, 1)

	// First run.
	result1, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if result1.PayoutsPosted != 1 {
		t.Errorf("first run: expected 1 payout, got %d", result1.PayoutsPosted)
	}

	// Second run on the same day - must post nothing.
	result2, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if result2.PayoutsPosted != 0 {
		t.Errorf("second run (idempotency): expected 0 payouts, got %d", result2.PayoutsPosted)
	}

	// Only 1 income transaction total.
	var txns []models.Transaction
	db.Where("account_id = ? AND type = ?", linkedAcct.ID, models.TransactionTypeIncome).Find(&txns)
	if len(txns) != 1 {
		t.Errorf("expected exactly 1 income transaction after 2 runs, got %d", len(txns))
	}
}

func TestProcessFixedDeposits_PayoutsStopAtMaturity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	start := date(2025, 1, 1)
	maturity := date(2025, 3, 1) // matures Mar 1
	nextPayout := date(2025, 2, 1)

	freq := models.PayoutFrequencyMonthly
	testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       120000,
		InterestRate:    12.0,
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypePeriodic,
		PayoutFrequency: &freq,
		LinkedAccountID: strPtr(linkedAcct.ID),
		NextPayoutDate:  timePtr(nextPayout),
	})

	// Run as of Jun 1 (well past maturity). Only 1 payout should post (Feb1; Mar1 == maturity
	// so Mar1 <= maturity but Apr would be > maturity).
	// Feb1 <= Jun1 AND Feb1 <= Mar1(maturity) -> post. nextPayout->Mar1.
	// Mar1 <= Jun1 AND Mar1 <= Mar1(maturity) -> post. nextPayout->Apr1.
	// Apr1 <= Jun1 BUT Apr1 > Mar1(maturity) -> stop.
	// So 2 payouts expected.
	asOf := date(2025, 6, 1)
	result, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}

	if result.PayoutsPosted != 2 {
		t.Errorf("expected 2 payouts (up to and including maturity date), got %d", result.PayoutsPosted)
	}
}

func TestProcessFixedDeposits_MaturityFlip(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)

	start := date(2024, 1, 1)
	maturity := date(2025, 1, 1)

	nextPayout := date(2025, 1, 1)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
	_, fd := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       100000,
		InterestRate:    5.0,
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypeCumulative, // cumulative, no payouts
		LinkedAccountID: strPtr(linkedAcct.ID),
		NextPayoutDate:  timePtr(nextPayout), // won't be used for cumulative
		Status:          models.FixedDepositStatusActive,
	})

	// Override: use a cumulative FD with maturity in the past.
	// Update it to be cumulative so the payout path skips it; the maturity flip should still fire.
	db.Model(&models.FixedDepositDetails{}).Where("account_id = ?", fd.AccountID).
		Update("payout_type", models.PayoutTypeCumulative)

	asOf := date(2025, 3, 1)
	result, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}

	if result.FDsMatured != 1 {
		t.Errorf("expected 1 FD matured, got %d", result.FDsMatured)
	}

	// Verify status flipped.
	var updatedFD models.FixedDepositDetails
	db.First(&updatedFD, "account_id = ?", fd.AccountID)
	if updatedFD.Status != models.FixedDepositStatusMatured {
		t.Errorf("expected status matured, got %s", updatedFD.Status)
	}
	if updatedFD.NextPayoutDate != nil {
		t.Errorf("expected NextPayoutDate cleared, got %v", updatedFD.NextPayoutDate)
	}

	// No income transactions should have been posted.
	var txns []models.Transaction
	db.Where("user_id = ? AND type = ?", user.ID, models.TransactionTypeIncome).Find(&txns)
	if len(txns) != 0 {
		t.Errorf("expected no income transactions for cumulative FD, got %d", len(txns))
	}
}

func TestProcessFixedDeposits_CumulativeFD_Untouched_By_Payouts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	catSvc := services.NewCategoryService(db)
	fdSvc := services.NewFixedDepositService(db, catSvc)

	user := testutil.CreateTestUser(t, db)
	linkedAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)

	start := date(2025, 1, 1)
	maturity := date(2026, 1, 1)

	testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{
		Principal:       100000,
		InterestRate:    5.0,
		StartDate:       start,
		MaturityDate:    maturity,
		PayoutType:      models.PayoutTypeCumulative,
		LinkedAccountID: strPtr(linkedAcct.ID),
		// No NextPayoutDate for cumulative
	})

	asOf := date(2025, 6, 1)
	result, err := fdSvc.ProcessFixedDeposits(asOf)
	if err != nil {
		t.Fatalf("ProcessFixedDeposits: %v", err)
	}

	if result.PayoutsPosted != 0 {
		t.Errorf("cumulative FD should not have payouts, got %d", result.PayoutsPosted)
	}

	// No income transactions.
	var txns []models.Transaction
	db.Where("account_id = ? AND type = ?", linkedAcct.ID, models.TransactionTypeIncome).Find(&txns)
	if len(txns) != 0 {
		t.Errorf("expected no income transactions for cumulative FD, got %d", len(txns))
	}
}

// date is a helper to construct a UTC midnight time.Time for the given y/m/d.
func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}
