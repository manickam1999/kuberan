package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// analyticsFixture is a MYR-base user with a MYR and a USD cash account and an
// expense category, all in the current UTC month.
type analyticsFixture struct {
	db         *gorm.DB
	userID     string
	myr, usd   *models.Account
	category   *models.Category
	monthStart time.Time
}

func newAnalyticsFixture(t *testing.T) *analyticsFixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	user := testutil.CreateTestUser(t, db) // base currency MYR (column default)
	myr := testutil.CreateTestCashAccount(t, db, user.ID)
	if err := db.Model(myr).Update("currency", "MYR").Error; err != nil {
		t.Fatal(err)
	}
	usd := testutil.CreateTestCashAccount(t, db, user.ID) // fixture accounts are USD
	now := time.Now().UTC()
	return &analyticsFixture{
		db: db, userID: user.ID, myr: myr, usd: usd,
		category:   testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense),
		monthStart: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
	}
}

// day returns 10:00 UTC of the n-th day (1-based) of the fixture month.
func (f *analyticsFixture) day(n int) time.Time {
	return f.monthStart.AddDate(0, 0, n-1).Add(10 * time.Hour)
}

func (f *analyticsFixture) add(t *testing.T, acct *models.Account, typ models.TransactionType, amount int64, at time.Time, personal *int64) *models.Transaction {
	t.Helper()
	tx := &models.Transaction{
		UserID: f.userID, AccountID: acct.ID, CategoryID: &f.category.ID,
		Type: typ, Amount: amount, Date: at, PersonalAmount: personal,
	}
	if err := f.db.Create(tx).Error; err != nil {
		t.Fatal(err)
	}
	return tx
}

// seedTwoRates makes USD/MYR 4.0 until day 10 and 5.0 from day 10 of the month.
func (f *analyticsFixture) seedTwoRates(t *testing.T) FXServicer {
	t.Helper()
	fxs := NewFXService(f.db, testFXStaleness)
	seedRate(t, fxs, "USD", "MYR", "4.0", f.monthStart)
	seedRate(t, fxs, "USD", "MYR", "5.0", f.monthStart.AddDate(0, 0, 9))
	return fxs
}

func (f *analyticsFixture) txSvc(fxs FXServicer) TransactionServicer {
	return NewTransactionService(f.db, NewAccountService(f.db), nil, WithTransactionFXService(fxs))
}

// seedMixedMonth: two 10.00 USD expenses on days 5 (rate 4.0) and 15 (rate 5.0),
// a 5.00 MYR expense on day 5 and a 2.00 USD income on day 15.
func (f *analyticsFixture) seedMixedMonth(t *testing.T) {
	t.Helper()
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(5), nil)
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(15), nil)
	f.add(t, f.myr, models.TransactionTypeExpense, 500, f.day(5), nil)
	f.add(t, f.usd, models.TransactionTypeIncome, 200, f.day(15), nil)
}

func TestAnalytics_MonthlyEqualsSumOfDailyConversions(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(f.seedTwoRates(t))
	f.seedMixedMonth(t)

	items, err := svc.GetMonthlySummary(f.userID, 1)
	testutil.AssertNoError(t, err)
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	// 1000*4.0 + 1000*5.0 + 500 = 9500 (one month-end conversion would give 10500).
	// Income 200 USD on day 15 = 1000.
	if items[0].Expenses != 9500 || items[0].Income != 1000 || items[0].Currency != "MYR" || items[0].FXMissing {
		t.Errorf("monthly = %+v, want expenses 9500 income 1000 MYR", items[0])
	}
}

func TestAnalytics_DailySpendingAndSummaryConvertPerDay(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(f.seedTwoRates(t))
	f.seedMixedMonth(t)

	spending, err := svc.GetDailySpending(f.userID, f.monthStart, f.monthStart.AddDate(0, 0, 19))
	testutil.AssertNoError(t, err)
	if spending[4].Total != 4500 || spending[14].Total != 5000 || spending[0].Total != 0 {
		t.Errorf("daily spending day5=%d day15=%d day1=%d, want 4500 5000 0", spending[4].Total, spending[14].Total, spending[0].Total)
	}
	if spending[4].Currency != "MYR" || spending[4].FXMissing {
		t.Errorf("day5 = %+v", spending[4])
	}

	summary, err := svc.GetDailySummary(f.userID, f.monthStart, f.monthStart.AddDate(0, 0, 19))
	testutil.AssertNoError(t, err)
	if summary[14].Expenses != 5000 || summary[14].Income != 1000 || summary[4].Expenses != 4500 {
		t.Errorf("daily summary day15 = %+v day5 = %+v", summary[14], summary[4])
	}
}

func TestAnalytics_SpendingByCategoryAndCashflowMixedCurrencies(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(f.seedTwoRates(t))
	f.seedMixedMonth(t)
	to := f.monthStart.AddDate(0, 1, 0).Add(-time.Second)

	sp, err := svc.GetSpendingByCategory(f.userID, f.monthStart, to)
	testutil.AssertNoError(t, err)
	if sp.TotalSpent != 9500 || len(sp.Items) != 1 || sp.Items[0].Total != 9500 || sp.Currency != "MYR" || sp.FXMissing || len(sp.Unconverted) != 0 {
		t.Errorf("spending = %+v", sp)
	}

	cf, err := svc.GetCashflow(f.userID, f.monthStart, to)
	testutil.AssertNoError(t, err)
	if cf.TotalExpenses != 9500 || cf.TotalIncome != 1000 || cf.Currency != "MYR" || cf.FXMissing {
		t.Errorf("cashflow = %+v", cf)
	}
}

func TestAnalytics_PersonalAmountAppliesBeforeConversion(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(f.seedTwoRates(t))
	personal := int64(400)
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(5), &personal) // 4.00 USD * 4.0

	sp, err := svc.GetSpendingByCategory(f.userID, f.monthStart, f.monthStart.AddDate(0, 1, 0).Add(-time.Second))
	testutil.AssertNoError(t, err)
	if sp.TotalSpent != 1600 {
		t.Errorf("total = %d, want 1600", sp.TotalSpent)
	}
}

func TestAnalytics_MissingRateIsExcludedNotOneToOne(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(NewFXService(f.db, testFXStaleness)) // no rates seeded
	f.seedMixedMonth(t)
	to := f.monthStart.AddDate(0, 1, 0).Add(-time.Second)

	monthly, err := svc.GetMonthlySummary(f.userID, 1)
	testutil.AssertNoError(t, err)
	if monthly[0].Expenses != 500 || monthly[0].Income != 0 || !monthly[0].FXMissing {
		t.Errorf("monthly = %+v, want only the MYR 500 and fx_missing", monthly[0])
	}

	daily, err := svc.GetDailySpending(f.userID, f.monthStart, f.monthStart.AddDate(0, 0, 19))
	testutil.AssertNoError(t, err)
	if daily[14].Total != 0 || !daily[14].FXMissing || daily[4].Total != 500 || !daily[4].FXMissing {
		t.Errorf("daily = day5 %+v day15 %+v", daily[4], daily[14])
	}

	sp, err := svc.GetSpendingByCategory(f.userID, f.monthStart, to)
	testutil.AssertNoError(t, err)
	if sp.TotalSpent != 500 || !sp.FXMissing || len(sp.Unconverted) != 1 ||
		sp.Unconverted[0].Currency != "USD" || sp.Unconverted[0].Amount != 2000 {
		t.Errorf("spending = %+v, want 500 with USD 2000 unconverted", sp)
	}

	cf, err := svc.GetCashflow(f.userID, f.monthStart, to)
	testutil.AssertNoError(t, err)
	if !cf.FXMissing || cf.TotalIncome != 0 || cf.TotalExpenses != 500 || len(cf.Unconverted) != 1 || cf.Unconverted[0].Amount != 2200 {
		t.Errorf("cashflow = %+v, want USD 2200 (2000 expense + 200 income) unconverted", cf)
	}
}

func TestAnalytics_NilFXServiceNeverConvertsOneToOne(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(nil)
	f.seedMixedMonth(t)

	monthly, err := svc.GetMonthlySummary(f.userID, 1)
	testutil.AssertNoError(t, err)
	if monthly[0].Expenses != 500 || !monthly[0].FXMissing {
		t.Errorf("monthly = %+v", monthly[0])
	}
}

func TestAnalytics_SingleCurrencyFastPathMakesNoFXQuery(t *testing.T) {
	f := newAnalyticsFixture(t)
	if err := f.db.Model(f.usd).Update("currency", "MYR").Error; err != nil {
		t.Fatal(err)
	}
	f.seedMixedMonth(t)
	to := f.monthStart.AddDate(0, 1, 0).Add(-time.Second)

	for name, fxs := range map[string]FXServicer{"nil fx": nil, "panicking fx": panicFXDated{}} {
		svc := f.txSvc(fxs)
		monthly, err := svc.GetMonthlySummary(f.userID, 1)
		testutil.AssertNoError(t, err)
		if monthly[0].Expenses != 2500 || monthly[0].Income != 200 || monthly[0].FXMissing {
			t.Errorf("%s: monthly = %+v", name, monthly[0])
		}
		if _, err := svc.GetDailySummary(f.userID, f.monthStart, to); err != nil {
			t.Fatal(err)
		}
		sp, err := svc.GetSpendingByCategory(f.userID, f.monthStart, to)
		testutil.AssertNoError(t, err)
		if sp.TotalSpent != 2500 || sp.FXMissing {
			t.Errorf("%s: spending = %+v", name, sp)
		}
		top, err := svc.GetTopExpenses(f.userID, f.monthStart, to, 2, nil)
		testutil.AssertNoError(t, err)
		if len(top.Items) != 2 || top.Items[0].AmountBase != 1000 || top.FXMissing {
			t.Errorf("%s: top = %+v", name, top)
		}
	}
}

func TestAnalytics_TopExpensesRankByConvertedAmount(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(f.seedTwoRates(t))
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(5), nil)  // 4000 MYR
	f.add(t, f.usd, models.TransactionTypeExpense, 1000, f.day(15), nil) // 5000 MYR
	f.add(t, f.myr, models.TransactionTypeExpense, 4500, f.day(6), nil)  // 4500 MYR

	top, err := svc.GetTopExpenses(f.userID, f.monthStart, f.monthStart.AddDate(0, 1, 0).Add(-time.Second), 2, nil)
	testutil.AssertNoError(t, err)
	if top.Currency != "MYR" || top.TotalCount != 3 || top.FXMissing || len(top.Items) != 2 {
		t.Fatalf("top = %+v", top)
	}
	first, second := top.Items[0], top.Items[1]
	if first.Amount != 1000 || first.AmountBase != 5000 || first.Currency != "USD" {
		t.Errorf("first = %+v, want native 1000 USD, base 5000", first)
	}
	if second.Amount != 4500 || second.AmountBase != 4500 || second.Currency != "MYR" {
		t.Errorf("second = %+v, want MYR 4500 (above the 4000 USD expense)", second)
	}
}

func TestAnalytics_TopExpensesMissingRateRanksLastAndFlags(t *testing.T) {
	f := newAnalyticsFixture(t)
	svc := f.txSvc(NewFXService(f.db, testFXStaleness))
	f.add(t, f.usd, models.TransactionTypeExpense, 9000, f.day(5), nil)
	f.add(t, f.myr, models.TransactionTypeExpense, 100, f.day(6), nil)

	top, err := svc.GetTopExpenses(f.userID, f.monthStart, f.monthStart.AddDate(0, 1, 0).Add(-time.Second), 5, nil)
	testutil.AssertNoError(t, err)
	if !top.FXMissing || len(top.Items) != 2 {
		t.Fatalf("top = %+v", top)
	}
	if top.Items[0].Currency != "MYR" || top.Items[0].FXMissing {
		t.Errorf("converted expense must rank first: %+v", top.Items[0])
	}
	if !top.Items[1].FXMissing || top.Items[1].AmountBase != 0 || top.Items[1].Amount != 9000 {
		t.Errorf("unconvertible expense must be flagged, not 1:1: %+v", top.Items[1])
	}
}
