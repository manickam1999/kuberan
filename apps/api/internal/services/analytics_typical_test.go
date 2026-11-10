package services

import (
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// monthDay returns noon UTC on `day` of the month `back` months before this one.
func monthDay(back, day int) time.Time {
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	return first.AddDate(0, -back, day-1)
}

func TestGetTypical_AveragesCompleteMonthsFromFirstActivity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	a := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 0)
	add := func(typ models.TransactionType, amount int64, at time.Time) {
		t.Helper()
		testutil.AssertNoError(t, db.Create(&models.Transaction{
			UserID: u.ID, AccountID: a.ID, Type: typ, Amount: amount, Date: at,
		}).Error)
	}
	// Two months of history; the opening balance, a transfer-free account and the
	// current month must not count.
	add(models.TransactionTypeOpeningBalance, 99999, monthDay(3, 1))
	add(models.TransactionTypeIncome, 5000, monthDay(1, 1))
	add(models.TransactionTypeExpense, 1000, monthDay(1, 1))
	add(models.TransactionTypeExpense, 2000, monthDay(1, 3))
	add(models.TransactionTypeIncome, 3000, monthDay(2, 1))
	add(models.TransactionTypeExpense, 500, monthDay(2, 2))
	add(models.TransactionTypeExpense, 77777, time.Now().UTC())

	got, err := NewTypicalService(db, nil).GetTypical(u.ID, 6)
	testutil.AssertNoError(t, err)

	if got.MonthsUsed != 2 || got.Months != 6 || got.Currency != "USD" || got.FXMissing {
		t.Fatalf("meta = %+v", got)
	}
	if got.AvgIncome != 4000 || got.AvgSpending != 1750 {
		t.Errorf("avg income/spending = %d/%d, want 4000/1750", got.AvgIncome, got.AvgSpending)
	}
	if got.SavedPercent != 56.3 {
		t.Errorf("saved = %v, want 56.3", got.SavedPercent)
	}
	if len(got.DailyCumulative) != 31 {
		t.Fatalf("curve length = %d", len(got.DailyCumulative))
	}
	for day, want := range map[int]int64{1: 500, 2: 750, 3: 1750, 31: 1750} {
		if got.DailyCumulative[day-1] != want {
			t.Errorf("curve[day %d] = %d, want %d (%v)", day, got.DailyCumulative[day-1], want, got.DailyCumulative)
		}
	}
	if len(got.ByCategory) != 1 || got.ByCategory[0].Total != 1750 {
		t.Errorf("by category = %+v", got.ByCategory)
	}
}

func TestGetTypical_NoHistoryAndValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	svc := NewTypicalService(db, nil)

	got, err := svc.GetTypical(u.ID, 6)
	testutil.AssertNoError(t, err)
	if got.MonthsUsed != 0 || got.AvgSpending != 0 || len(got.DailyCumulative) != 31 || got.ByCategory == nil || got.Unconverted == nil {
		t.Errorf("empty = %+v", got)
	}
	for _, m := range []int{0, 25} {
		_, err = svc.GetTypical(u.ID, m)
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	}
}

func TestGetTypical_ScopedToUserAndFlagsMissingRates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	other := testutil.CreateTestUserWithEmail(t, db, "other@example.com")
	mine := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 0)
	eur := p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 0), "EUR")
	theirs := testutil.CreateTestCashAccountWithBalance(t, db, other.ID, 0)
	for _, tx := range []models.Transaction{
		{UserID: u.ID, AccountID: mine.ID, Type: models.TransactionTypeExpense, Amount: 1000, Date: monthDay(1, 2)},
		{UserID: u.ID, AccountID: eur.ID, Type: models.TransactionTypeExpense, Amount: 5000, Date: monthDay(1, 2)},
		{UserID: other.ID, AccountID: theirs.ID, Type: models.TransactionTypeExpense, Amount: 900000, Date: monthDay(1, 2)},
	} {
		testutil.AssertNoError(t, db.Create(&tx).Error)
	}

	got, err := NewTypicalService(db, NewFXService(db, testFXStaleness)).GetTypical(u.ID, 3)
	testutil.AssertNoError(t, err)
	if got.AvgSpending != 1000 || !got.FXMissing {
		t.Errorf("typical = %+v, want only the USD 1000 and fx_missing", got)
	}
	if len(got.Unconverted) != 1 || got.Unconverted[0].Currency != "EUR" {
		t.Errorf("unconverted = %+v", got.Unconverted)
	}
}
