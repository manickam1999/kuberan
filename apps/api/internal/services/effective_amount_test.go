package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"

	"gorm.io/gorm"
)

// Tests in this file set personal_amount directly on expense rows to prove that
// every spending aggregate reads the effective amount COALESCE(personal_amount, amount)
// (plan 021, D1). Behaviour with personal_amount NULL is covered by the existing tests.

// sharedFixture holds one shared expense (amount 6000, personal 2000) and one
// ordinary expense (amount 1000) in the same category.
type sharedFixture struct {
	db       *gorm.DB
	txSvc    TransactionServicer
	userID   string
	catID    string
	sharedID string
	when     time.Time
}

func newSharedFixture(t *testing.T, when time.Time) *sharedFixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	shared, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 6000, "shared dinner", when)
	testutil.AssertNoError(t, err)
	setPersonalAmount(t, db, shared.ID, 2000)
	_, err = txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "coffee", when.Add(time.Minute))
	testutil.AssertNoError(t, err)

	return &sharedFixture{db: db, txSvc: txSvc, userID: user.ID, catID: cat.ID, sharedID: shared.ID, when: when}
}

func setPersonalAmount(t *testing.T, db *gorm.DB, txnID string, personal int64) {
	t.Helper()
	if err := db.Model(&models.Transaction{}).Where("id = ?", txnID).UpdateColumn("personal_amount", personal).Error; err != nil {
		t.Fatalf("failed to set personal_amount: %v", err)
	}
}

func TestEffectiveAmount_SpendingByCategory(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	f := newSharedFixture(t, from.Add(time.Hour))

	res, err := f.txSvc.GetSpendingByCategory(f.userID, from, to)
	testutil.AssertNoError(t, err)
	if res.TotalSpent != 3000 {
		t.Errorf("total spent = %d, want 3000 (2000 personal + 1000)", res.TotalSpent)
	}
	if len(res.Items) != 1 || res.Items[0].Total != 3000 {
		t.Errorf("expected single category total 3000, got %+v", res.Items)
	}
}

func TestEffectiveAmount_Cashflow(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	f := newSharedFixture(t, from.Add(time.Hour))

	res, err := f.txSvc.GetCashflow(f.userID, from, to)
	testutil.AssertNoError(t, err)
	if res.TotalExpenses != 3000 {
		t.Errorf("total expenses = %d, want 3000", res.TotalExpenses)
	}
	if len(res.Expenses) != 1 || res.Expenses[0].Total != 3000 {
		t.Errorf("expected single expense category total 3000, got %+v", res.Expenses)
	}
}

func TestEffectiveAmount_ZeroPersonalContributesNothing(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	f := newSharedFixture(t, from.Add(time.Hour))
	setPersonalAmount(t, f.db, f.sharedID, 0)

	res, err := f.txSvc.GetSpendingByCategory(f.userID, from, to)
	testutil.AssertNoError(t, err)
	if res.TotalSpent != 1000 {
		t.Errorf("total spent = %d, want 1000 (a 0 personal amount must not fall back to amount)", res.TotalSpent)
	}
}

func TestEffectiveAmount_MonthlySummary(t *testing.T) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	f := newSharedFixture(t, monthStart.Add(time.Hour))

	items, err := f.txSvc.GetMonthlySummary(f.userID, 1)
	testutil.AssertNoError(t, err)
	if len(items) != 1 || items[0].Expenses != 3000 {
		t.Errorf("expected expenses 3000, got %+v", items)
	}
}

func TestEffectiveAmount_DailySpendingAndSummary(t *testing.T) {
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	f := newSharedFixture(t, day.Add(10*time.Hour))

	spending, err := f.txSvc.GetDailySpending(f.userID, day, day)
	testutil.AssertNoError(t, err)
	if len(spending) != 1 || spending[0].Total != 3000 {
		t.Errorf("daily spending: expected 3000, got %+v", spending)
	}

	summary, err := f.txSvc.GetDailySummary(f.userID, day, day)
	testutil.AssertNoError(t, err)
	if len(summary) != 1 || summary[0].Expenses != 3000 {
		t.Errorf("daily summary: expected expenses 3000, got %+v", summary)
	}
}

func TestEffectiveAmount_TopExpenses(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	t.Run("orders_by_effective_amount_and_keeps_full_amount", func(t *testing.T) {
		f := newSharedFixture(t, from.Add(time.Hour))
		// Unshared expenses of 2500 and 1500 bracket the shared expense (amount 6000,
		// effective 2000): ordering by effective amount gives 2500, shared, 1500, 1000,
		// whereas ordering by raw amount would put the shared expense first.
		acct := testutil.CreateTestCashAccountWithBalance(t, f.db, f.userID, 100000)
		big, err := f.txSvc.CreateTransaction(f.userID, acct.ID, &f.catID, models.TransactionTypeExpense, 2500, "big", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		mid, err := f.txSvc.CreateTransaction(f.userID, acct.ID, &f.catID, models.TransactionTypeExpense, 1500, "mid", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		res, err := f.txSvc.GetTopExpenses(f.userID, from, to, 10, nil)
		testutil.AssertNoError(t, err)
		if len(res.Items) != 4 {
			t.Fatalf("expected 4 items, got %d", len(res.Items))
		}
		gotIDs := []string{res.Items[0].ID, res.Items[1].ID, res.Items[2].ID}
		wantIDs := []string{big.ID, f.sharedID, mid.ID}
		for i := range wantIDs {
			if gotIDs[i] != wantIDs[i] {
				t.Fatalf("order mismatch at %d: got descriptions %q, %q, %q, %q; want big(2500), shared(eff 2000), mid(1500), coffee(1000)",
					i, res.Items[0].Description, res.Items[1].Description, res.Items[2].Description, res.Items[3].Description)
			}
		}
		if res.Items[3].Description != "coffee" || res.Items[3].Amount != 1000 {
			t.Errorf("fourth item = %+v, want the 1000 coffee", res.Items[3])
		}
		if res.Items[0].Amount != 2500 || res.Items[0].PersonalAmount != nil {
			t.Errorf("first item = %+v, want unshared 2500", res.Items[0])
		}
		if res.Items[1].Amount != 6000 {
			t.Errorf("amount = %d, want full amount 6000", res.Items[1].Amount)
		}
		if res.Items[1].PersonalAmount == nil || *res.Items[1].PersonalAmount != 2000 {
			t.Errorf("personal_amount = %v, want 2000", res.Items[1].PersonalAmount)
		}
		if res.Items[2].Amount != 1500 || res.Items[2].PersonalAmount != nil {
			t.Errorf("third item = %+v, want unshared 1500", res.Items[2])
		}
	})

	t.Run("excludes_zero_effective_amount", func(t *testing.T) {
		f := newSharedFixture(t, from.Add(time.Hour))
		setPersonalAmount(t, f.db, f.sharedID, 0)

		res, err := f.txSvc.GetTopExpenses(f.userID, from, to, 10, nil)
		testutil.AssertNoError(t, err)
		if len(res.Items) != 1 || res.Items[0].Amount != 1000 {
			t.Errorf("expected only the 1000 coffee, got %+v", res.Items)
		}
	})
}

func TestEffectiveAmount_BudgetProgress(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	budgetSvc := NewBudgetService(db)
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

	// Mid-month-safe timestamp inside the current monthly window.
	now := time.Now()
	when := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location())

	shared, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 6000, "dinner", when)
	testutil.AssertNoError(t, err)
	setPersonalAmount(t, db, shared.ID, 2000)

	single, err := budgetSvc.GetBudgetProgress(user.ID, budget.ID)
	testutil.AssertNoError(t, err)
	if single.Spent != 2000 {
		t.Errorf("single progress spent = %d, want 2000", single.Spent)
	}

	batch, err := budgetSvc.GetActiveBudgetsProgress(user.ID)
	testutil.AssertNoError(t, err)
	if len(batch) != 1 || batch[0].Spent != 2000 {
		t.Errorf("batch progress = %+v, want spent 2000", batch)
	}
}
