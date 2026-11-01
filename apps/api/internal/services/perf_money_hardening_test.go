package services

import (
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestAggregateRangeCaps(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewTransactionService(db, NewAccountService(db), nil)
	user := testutil.CreateTestUser(t, db)

	for _, months := range []int{0, -1, 25} {
		_, err := svc.GetMonthlySummary(user.ID, months)
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	}
	items, err := svc.GetMonthlySummary(user.ID, 24)
	testutil.AssertNoError(t, err)
	if len(items) != 24 {
		t.Errorf("expected 24 months, got %d", len(items))
	}

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ok := from.AddDate(0, 0, 366)
	tooLong := from.AddDate(0, 0, 367)

	daily, err := svc.GetDailySpending(user.ID, from, ok)
	testutil.AssertNoError(t, err)
	if len(daily) != 367 {
		t.Errorf("expected 367 daily buckets, got %d", len(daily))
	}
	_, err = svc.GetDailySpending(user.ID, from, tooLong)
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.GetDailySummary(user.ID, from, tooLong)
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)

	// to before from stays an empty (non-nil) result.
	empty, err := svc.GetDailySummary(user.ID, ok, from)
	testutil.AssertNoError(t, err)
	if empty == nil || len(empty) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", empty)
	}
}

func TestDailySummary_GroupedBucketsAndInitialBalance(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewTransactionService(db, NewAccountService(db), nil)
	user := testutil.CreateTestUser(t, db)
	acct := testutil.CreateTestCashAccount(t, db, user.ID)

	day1 := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 3, 23, 30, 0, 0, time.UTC)
	personal := int64(400)
	rows := []models.Transaction{
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeIncome, Amount: 5000, Description: "Initial balance", Date: day1},
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeIncome, Amount: 1000, Description: "Salary", Date: day1},
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 300, Description: "Lunch", Date: day1},
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 1000, PersonalAmount: &personal, Description: "Shared dinner", Date: day3},
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeExpense, Amount: 200, Description: "Snack", Date: day3},
		{UserID: user.ID, AccountID: acct.ID, Type: models.TransactionTypeTransfer, Amount: 999, Description: "Ignored", Date: day3},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	items, err := svc.GetDailySummary(user.ID, day1, day3)
	testutil.AssertNoError(t, err)
	want := []DailySummaryItem{
		{Date: "2026-03-01", Income: 1000, Expenses: 300},
		{Date: "2026-03-02", Income: 0, Expenses: 0},
		{Date: "2026-03-03", Income: 0, Expenses: 600},
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(items), len(want), items)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, items[i], want[i])
		}
	}

	spending, err := svc.GetDailySpending(user.ID, day1, day3)
	testutil.AssertNoError(t, err)
	if spending[0].Total != 300 || spending[1].Total != 0 || spending[2].Total != 600 {
		t.Errorf("unexpected daily spending: %+v", spending)
	}
}

func TestCreateTransfer_RejectsCurrencyMismatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewTransactionService(db, NewAccountService(db), nil)
	user := testutil.CreateTestUser(t, db)
	usd := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
	eur := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 0)
	if err := db.Model(&models.Account{}).Where("id = ?", eur.ID).Update("currency", "EUR").Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.CreateTransfer(user.ID, usd.ID, eur.ID, 500, "fx", time.Now())
	testutil.AssertAppError(t, err, apperrors.ErrTransferCurrencyMismatch.Code)

	// Balances are untouched.
	var got models.Account
	if err := db.First(&got, "id = ?", usd.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Balance != 10000 {
		t.Errorf("balance changed to %d", got.Balance)
	}
}

func TestInvestmentRoundingAndOverflowGuard(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	svc := NewInvestmentService(db, acctSvc)
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	sec := testutil.CreateTestSecurity(t, db)

	// 0.29 * 100 = 28.999999999999996 in float64; truncation would give 28.
	inv, err := svc.AddInvestment(user.ID, account.ID, sec.ID, 0.29, 100, "", nil, 0, "")
	testutil.AssertNoError(t, err)
	if inv.CostBasis != 29 {
		t.Errorf("cost basis = %d, want 29", inv.CostBasis)
	}

	buy, err := svc.RecordBuy(user.ID, inv.ID, time.Now(), 1.5, 3, 0, "", "")
	testutil.AssertNoError(t, err)
	if buy.TotalAmount != 5 { // 4.5 rounds half away from zero
		t.Errorf("buy total = %d, want 5", buy.TotalAmount)
	}

	_, err = svc.AddInvestment(user.ID, account.ID, sec.ID, 1e12, 1_000_000, "", nil, 0, "")
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.RecordBuy(user.ID, inv.ID, time.Now(), 1e12, 1_000_000, 0, "", "")
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
}

func TestRecordSell_RevalidatesLockedQuantity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewInvestmentService(db, NewAccountService(db))
	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	sec := testutil.CreateTestSecurity(t, db)
	inv := testutil.CreateTestInvestment(t, db, account.ID, sec.ID) // qty 10, cost 100000

	_, err := svc.RecordSell(user.ID, inv.ID, time.Now(), 6, 20000, 0, "", "")
	testutil.AssertNoError(t, err)
	_, err = svc.RecordSell(user.ID, inv.ID, time.Now(), 6, 20000, 0, "", "")
	testutil.AssertAppError(t, err, apperrors.ErrInsufficientShares.Code)

	var got models.Investment
	if err := db.First(&got, "id = ?", inv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 4 || got.CostBasis != 40000 {
		t.Errorf("investment = qty %v cost %d, want qty 4 cost 40000", got.Quantity, got.CostBasis)
	}
}

func TestRecordPrices_BatchedSkipsDuplicates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewSecurityService(db)
	sec := testutil.CreateTestSecurity(t, db)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, t1)

	n, err := svc.RecordPrices([]SecurityPriceInput{
		{SecurityID: sec.ID, Price: 101, RecordedAt: t1}, // already stored
		{SecurityID: sec.ID, Price: 102, RecordedAt: t2},
		{SecurityID: sec.ID, Price: 103, RecordedAt: t2}, // duplicate within the batch
	})
	testutil.AssertNoError(t, err)
	if n != 1 {
		t.Errorf("recorded = %d, want 1", n)
	}
	var count int64
	db.Model(&models.SecurityPrice{}).Where("security_id = ?", sec.ID).Count(&count)
	if count != 2 {
		t.Errorf("rows = %d, want 2", count)
	}
}

func TestLatestPrices_PicksNewestPerSecurity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	a := testutil.CreateTestSecurityWithParams(t, db, "AAA", "A", models.AssetTypeStock, "X")
	b := testutil.CreateTestSecurityWithParams(t, db, "BBB", "B", models.AssetTypeStock, "X")
	c := testutil.CreateTestSecurityWithParams(t, db, "CCC", "C", models.AssetTypeStock, "X")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	testutil.CreateTestSecurityPrice(t, db, a.ID, 100, base)
	testutil.CreateTestSecurityPrice(t, db, a.ID, 300, base.Add(2*time.Hour))
	testutil.CreateTestSecurityPrice(t, db, a.ID, 200, base.Add(time.Hour))
	testutil.CreateTestSecurityPrice(t, db, b.ID, 50, base)

	prices, err := getLatestPrices(db, []string{a.ID, b.ID, c.ID, a.ID})
	testutil.AssertNoError(t, err)
	if prices[a.ID] != 300 || prices[b.ID] != 50 {
		t.Errorf("prices = %v", prices)
	}
	if _, ok := prices[c.ID]; ok {
		t.Errorf("security without prices must be absent: %v", prices)
	}
}

func TestComputeAndRecordSnapshots_SetBasedUpsert(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewPortfolioSnapshotService(db)

	u1 := testutil.CreateTestUser(t, db)
	u2 := testutil.CreateTestUserWithEmail(t, db, "second@example.com")
	testutil.CreateTestCashAccountWithBalance(t, db, u1.ID, 10000)
	testutil.CreateTestCreditCardAccount(t, db, u1.ID, 2500)
	inv := testutil.CreateTestInvestmentAccount(t, db, u1.ID)
	sec := testutil.CreateTestSecurity(t, db)
	holding := testutil.CreateTestInvestment(t, db, inv.ID, sec.ID) // qty 10
	if err := db.Model(&models.Investment{}).Where("id = ?", holding.ID).Update("quantity", 0.29).Error; err != nil {
		t.Fatal(err)
	}
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 100, time.Now().Add(-time.Hour))
	testutil.CreateTestCashAccountWithBalance(t, db, u2.ID, 700)

	at := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	n, err := svc.ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	if n != 2 {
		t.Fatalf("recorded = %d, want 2", n)
	}

	// Re-run after a balance change: rows are updated, not duplicated.
	if err := db.Model(&models.Account{}).Where("user_id = ? AND type = ?", u2.ID, models.AccountTypeCash).
		Update("balance", 900).Error; err != nil {
		t.Fatal(err)
	}
	n, err = svc.ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	if n != 2 {
		t.Fatalf("recorded = %d, want 2", n)
	}

	var snaps []models.PortfolioSnapshot
	if err := db.Order("user_id").Find(&snaps).Error; err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	byUser := map[string]models.PortfolioSnapshot{}
	for _, s := range snaps {
		byUser[s.UserID] = s
	}
	s1 := byUser[u1.ID]
	// 0.29 * 100 rounds to 29 (truncation would give 28).
	if s1.CashBalance != 10000 || s1.DebtBalance != 2500 || s1.InvestmentValue != 29 || s1.TotalNetWorth != 10000+29-2500 {
		t.Errorf("u1 snapshot = %+v", s1)
	}
	if s2 := byUser[u2.ID]; s2.CashBalance != 900 || s2.TotalNetWorth != 900 {
		t.Errorf("u2 snapshot = %+v", s2)
	}
}

func TestGetGroupedSnapshots_RangeCap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewPortfolioSnapshotService(db)
	user := testutil.CreateTestUser(t, db)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := svc.GetGroupedSnapshots(user.ID, from, from.AddDate(0, 0, 94), "hour")
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.GetGroupedSnapshots(user.ID, from, from.AddDate(11, 0, 0), "day")
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.GetGroupedSnapshots(user.ID, from, from.AddDate(10, 0, 0), "day")
	testutil.AssertNoError(t, err)
}
