package services

import (
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func (f *analyticsFixture) budget(t *testing.T, svc BudgetServicer, amount int64, opts ...BudgetOption) *models.Budget {
	t.Helper()
	b, err := svc.CreateBudget(f.userID, f.category.ID, "Food", amount, models.BudgetPeriodMonthly, opts...)
	testutil.AssertNoError(t, err)
	return b
}

func TestBudgetCurrency_DefaultsToBaseAndCanBeSetAndChanged(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := NewBudgetService(f.db)

	b := f.budget(t, svc, 10000)
	if b.Currency != "MYR" {
		t.Errorf("default currency = %q, want the user's base MYR", b.Currency)
	}
	if err := f.db.Delete(b).Error; err != nil {
		t.Fatal(err)
	}

	b = f.budget(t, svc, 10000, WithBudgetCurrency("usd"))
	if b.Currency != "USD" {
		t.Errorf("explicit currency = %q, want USD", b.Currency)
	}

	updated, err := svc.UpdateBudget(f.userID, b.ID, "", nil, nil, nil, WithBudgetCurrency("SGD"))
	testutil.AssertNoError(t, err)
	reloaded, err := svc.GetBudgetByID(f.userID, updated.ID)
	testutil.AssertNoError(t, err)
	if reloaded.Currency != "SGD" {
		t.Errorf("updated currency = %q, want SGD", reloaded.Currency)
	}

	// An update without a currency leaves it alone.
	if _, err := svc.UpdateBudget(f.userID, b.ID, "Renamed", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = svc.GetBudgetByID(f.userID, b.ID)
	if reloaded.Currency != "SGD" {
		t.Errorf("currency changed by an unrelated update: %q", reloaded.Currency)
	}

	if _, err := svc.CreateBudget(f.userID, f.category.ID, "Bad", 100, models.BudgetPeriodYearly, WithBudgetCurrency("US")); err == nil {
		t.Error("an invalid currency must be rejected")
	}
}

func TestBudgetProgress_SingleCurrencyFastPathNeedsNoFX(t *testing.T) {
	f := newAnalyticsFixture(t)
	for name, fxs := range map[string]FXServicer{"nil fx": nil, "panicking fx": panicFXDated{}} {
		svc := NewBudgetService(f.db, WithBudgetFXService(fxs))
		b := f.budget(t, svc, 10000, WithBudgetCurrency("MYR"))
		tx := f.add(t, f.myr, models.TransactionTypeExpense, 2500, f.day(2), nil)

		p, err := svc.GetBudgetProgress(f.userID, b.ID)
		testutil.AssertNoError(t, err)
		if p.Spent != 2500 || p.Remaining != 7500 || p.Currency != "MYR" || p.FXMissing {
			t.Errorf("%s: progress = %+v", name, p)
		}
		all, err := svc.GetActiveBudgetsProgress(f.userID)
		testutil.AssertNoError(t, err)
		if len(all) != 1 || all[0].Spent != 2500 || all[0].Currency != "MYR" {
			t.Errorf("%s: active progress = %+v", name, all)
		}
		// reset for the next iteration
		f.db.Unscoped().Delete(tx)
		f.db.Delete(b)
	}
}

func TestBudgetProgress_ConvertsSpendingToBudgetCurrencyPerDay(t *testing.T) {
	f := newAnalyticsFixture(t)
	fxs := f.seedTwoRates(t) // USD/MYR 4.0 before day 10, 5.0 from day 10
	svc := NewBudgetService(f.db, WithBudgetFXService(fxs))
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(5), nil)  // 10.00 USD
	f.add(t, f.myr, models.TransactionTypeExpense, 4000, f.day(5), nil)  // 40.00 MYR at 4.0 = 10.00 USD
	f.add(t, f.myr, models.TransactionTypeExpense, 5000, f.day(15), nil) // 50.00 MYR at 5.0 = 10.00 USD

	usdBudget := f.budget(t, svc, 5000, WithBudgetCurrency("USD"))
	p, err := svc.GetBudgetProgress(f.userID, usdBudget.ID)
	testutil.AssertNoError(t, err)
	// Each MYR expense converts at its own day's rate: 1000 + 1000 + 1000.
	if p.Spent != 3000 || p.Remaining != 2000 || p.Currency != "USD" || p.FXMissing {
		t.Errorf("USD progress = %+v, want spent 3000", p)
	}
	if p.Percentage != 60 {
		t.Errorf("percentage = %v, want 60", p.Percentage)
	}
}

func TestBudgetProgress_ActiveBudgetsMixedCurrencies(t *testing.T) {
	f := newAnalyticsFixture(t)
	fxs := f.seedTwoRates(t)
	svc := NewBudgetService(f.db, WithBudgetFXService(fxs))
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(5), nil)
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(15), nil)
	f.add(t, f.myr, models.TransactionTypeExpense, 500, f.day(5), nil)

	other := testutil.CreateTestCategory(t, f.db, f.userID, models.CategoryTypeExpense)
	usdBudget := f.budget(t, svc, 5000, WithBudgetCurrency("USD"))
	myrBudgetOther, err := svc.CreateBudget(f.userID, other.ID, "Other", 20000, models.BudgetPeriodMonthly)
	testutil.AssertNoError(t, err)
	f.db.Create(&models.Transaction{UserID: f.userID, AccountID: f.usd.ID, CategoryID: &other.ID,
		Type: models.TransactionTypeExpense, Amount: 300, Date: f.day(15)})

	all, err := svc.GetActiveBudgetsProgress(f.userID)
	testutil.AssertNoError(t, err)
	byID := map[string]BudgetProgress{}
	for _, p := range all {
		byID[p.BudgetID] = p
	}
	// USD budget: 1000 + 1000 USD + 500 MYR at 4.0 = 125 USD cents.
	if p := byID[usdBudget.ID]; p.Spent != 2125 || p.Currency != "USD" || p.FXMissing {
		t.Errorf("USD budget = %+v, want spent 2125", p)
	}
	// MYR budget on the other category: 300 USD at 5.0 = 1500 MYR.
	if p := byID[myrBudgetOther.ID]; p.Spent != 1500 || p.Currency != "MYR" || p.FXMissing {
		t.Errorf("MYR budget = %+v, want spent 1500", p)
	}
}

func TestBudgetProgress_MissingRateIsExcludedAndFlagged(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := NewBudgetService(f.db, WithBudgetFXService(NewFXService(f.db, testFXStaleness)))
	f.add(t, f.usd, models.TransactionTypeExpense, 9999, f.day(5), nil)
	f.add(t, f.myr, models.TransactionTypeExpense, 700, f.day(5), nil)
	b := f.budget(t, svc, 10000) // MYR

	p, err := svc.GetBudgetProgress(f.userID, b.ID)
	testutil.AssertNoError(t, err)
	if p.Spent != 700 || !p.FXMissing {
		t.Errorf("progress = %+v, want 700 with fx_missing", p)
	}
	all, err := svc.GetActiveBudgetsProgress(f.userID)
	testutil.AssertNoError(t, err)
	if len(all) != 1 || all[0].Spent != 700 || !all[0].FXMissing {
		t.Errorf("active = %+v", all)
	}

	// With no FX service at all the foreign spending is also left out.
	nilSvc := NewBudgetService(f.db)
	p, err = nilSvc.GetBudgetProgress(f.userID, b.ID)
	testutil.AssertNoError(t, err)
	if p.Spent != 700 || !p.FXMissing {
		t.Errorf("nil fx progress = %+v", p)
	}
}
