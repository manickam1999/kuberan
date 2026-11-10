package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func newDemoTestService(t *testing.T) (*demoService, *gorm.DB) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	svc := NewDemoService(db, WithDemoClock(func() time.Time {
		return time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	}))
	d, ok := svc.(*demoService)
	if !ok {
		t.Fatal("unexpected service type")
	}
	return d, db
}

func countRows(t *testing.T, db *gorm.DB, model any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Unscoped().Model(model).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestDemoLoadBuildsConsistentHousehold(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)

	res, err := svc.Load(user.ID)
	testutil.AssertNoError(t, err)
	if res.Total == 0 || res.Rows["accounts"] < 5 || res.Rows["transactions"] < 60 {
		t.Fatalf("unexpectedly small household: %+v", res)
	}
	var recorded int64
	db.Model(&models.DemoRow{}).Where("user_id = ?", user.ID).Count(&recorded)
	if int(recorded) != res.Total {
		t.Fatalf("demo_rows has %d rows, result says %d", recorded, res.Total)
	}

	// Every cash and card balance equals what its transactions add up to.
	var accounts []models.Account
	db.Where("user_id = ?", user.ID).Find(&accounts)
	for _, a := range accounts {
		if a.Type == models.AccountTypeInvestment {
			continue
		}
		var txns []models.Transaction
		db.Where("account_id = ? OR to_account_id = ?", a.ID, a.ID).Find(&txns)
		var sum int64
		for _, tx := range txns {
			switch {
			case tx.Type == models.TransactionTypeTransfer && tx.AccountID == a.ID:
				d, _ := balanceDelta(a.Type, models.TransactionTypeExpense, tx.Amount)
				sum += d
			case tx.Type == models.TransactionTypeTransfer:
				d, _ := balanceDelta(a.Type, models.TransactionTypeIncome, tx.EffectiveToAmount())
				sum += d
			default:
				d, _ := balanceDelta(a.Type, tx.Type, tx.Amount)
				sum += d
			}
		}
		if sum != a.Balance {
			t.Errorf("%s balance %d, transactions add up to %d", a.Name, a.Balance, sum)
		}
		if a.Type == models.AccountTypeCash && a.Balance < 0 {
			t.Errorf("%s is overdrawn: %d", a.Name, a.Balance)
		}
		if a.Type == models.AccountTypeCreditCard && (a.Balance <= 0 || a.Balance > a.CreditLimit) {
			t.Errorf("%s owes %d against a limit of %d", a.Name, a.Balance, a.CreditLimit)
		}
	}

	// A holding with prices, a budget and receivables exist; shared expenses keep the invariant.
	if res.Rows["investments"] != len(demoHoldings) || res.Rows["budgets"] != len(demoBudgets) || res.Rows["receivables"] != 3 {
		t.Errorf("rows = %+v", res.Rows)
	}
	var priced int64
	db.Model(&models.SecurityPrice{}).Count(&priced)
	if priced == 0 {
		t.Error("no sample prices")
	}
	var bad int64
	db.Model(&models.Transaction{}).Where("user_id = ? AND personal_amount IS NOT NULL AND personal_amount > amount", user.ID).Count(&bad)
	if bad != 0 {
		t.Errorf("%d transactions with personal_amount above amount", bad)
	}
	// Nothing is dated in the future.
	var future int64
	db.Model(&models.Transaction{}).Where("user_id = ? AND date > ?", user.ID, time.Date(2026, 10, 28, 23, 0, 0, 0, time.UTC)).Count(&future)
	if future != 0 {
		t.Errorf("%d future transactions", future)
	}
}

func TestDemoLoadIsDeterministic(t *testing.T) {
	svc, db := newDemoTestService(t)
	a, b := testutil.CreateTestUser(t, db), testutil.CreateTestUser(t, db)
	ra, err := svc.Load(a.ID)
	testutil.AssertNoError(t, err)
	rb, err := svc.Load(b.ID)
	testutil.AssertNoError(t, err)
	// The second user reuses the shared sample securities, so only those differ.
	if ra.Rows["transactions"] != rb.Rows["transactions"] || ra.Rows["accounts"] != rb.Rows["accounts"] {
		t.Fatalf("not deterministic: %+v vs %+v", ra.Rows, rb.Rows)
	}
	if rb.Rows["securities"] != 0 {
		t.Errorf("second user created securities: %d", rb.Rows["securities"])
	}
	var sumA, sumB int64
	db.Model(&models.Transaction{}).Where("user_id = ?", a.ID).Select("COALESCE(SUM(amount),0)").Scan(&sumA)
	db.Model(&models.Transaction{}).Where("user_id = ?", b.ID).Select("COALESCE(SUM(amount),0)").Scan(&sumB)
	if sumA != sumB {
		t.Errorf("amount sums differ: %d vs %d", sumA, sumB)
	}
}

func TestDemoLoadRefusals(t *testing.T) {
	svc, db := newDemoTestService(t)

	t.Run("already loaded", func(t *testing.T) {
		u := testutil.CreateTestUser(t, db)
		_, err := svc.Load(u.ID)
		testutil.AssertNoError(t, err)
		_, err = svc.Load(u.ID)
		testutil.AssertAppError(t, err, "DEMO_ALREADY_LOADED")
	})
	t.Run("own transactions", func(t *testing.T) {
		u := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestCashAccount(t, db, u.ID)
		testutil.CreateTestTransaction(t, db, u.ID, acct.ID, models.TransactionTypeExpense, 500)
		_, err := svc.Load(u.ID)
		testutil.AssertAppError(t, err, "DEMO_HAS_DATA")
		if n := countRows(t, db, &models.DemoRow{}, "user_id = ?", u.ID); n != 0 {
			t.Errorf("refused load left %d demo rows", n)
		}
	})
	t.Run("opening balances do not count", func(t *testing.T) {
		u := testutil.CreateTestUser(t, db)
		acct := testutil.CreateTestCashAccount(t, db, u.ID)
		db.Create(&models.Transaction{UserID: u.ID, AccountID: acct.ID, Type: models.TransactionTypeOpeningBalance, Amount: 1000, Date: time.Now()})
		_, err := svc.Load(u.ID)
		testutil.AssertNoError(t, err)
	})
}

func TestDemoRemoveDeletesOnlyDemoRows(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)
	other := testutil.CreateTestUser(t, db)

	// Real data that exists before and must survive: an account with an opening
	// balance, and a category named like a sample one (reused, not removed).
	realAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 5000)
	db.Create(&models.Transaction{UserID: user.ID, AccountID: realAcct.ID, Type: models.TransactionTypeOpeningBalance, Amount: 5000, Date: time.Now()})
	groceries := &models.Category{UserID: user.ID, Name: "Groceries", Type: models.CategoryTypeExpense}
	db.Create(groceries)

	_, err := svc.Load(user.ID)
	testutil.AssertNoError(t, err)
	_, err = svc.Load(other.ID)
	testutil.AssertNoError(t, err)

	var demoCats int64
	db.Model(&models.DemoRow{}).Where("user_id = ? AND table_name = ?", user.ID, "categories").Count(&demoCats)
	if int(demoCats) != len(demoCategories)-1 {
		t.Errorf("the existing Groceries category must be reused: %d demo categories", demoCats)
	}

	// The user's own category use is cleared, not blocked, and their own
	// transactions on real accounts are untouched.
	ownTx := &models.Transaction{UserID: user.ID, AccountID: realAcct.ID, CategoryID: &groceries.ID, Type: models.TransactionTypeExpense, Amount: 100, Date: time.Now()}
	db.Create(ownTx)

	res, err := svc.Remove(user.ID)
	testutil.AssertNoError(t, err)
	if res.Total == 0 {
		t.Fatal("nothing removed")
	}

	if n := countRows(t, db, &models.Account{}, "user_id = ?", user.ID); n != 1 {
		t.Errorf("user should keep only the real account, has %d", n)
	}
	if n := countRows(t, db, &models.Transaction{}, "user_id = ?", user.ID); n != 2 {
		t.Errorf("user should keep 2 transactions, has %d", n)
	}
	if n := countRows(t, db, &models.Category{}, "user_id = ?", user.ID); n != 1 {
		t.Errorf("user should keep only Groceries, has %d", n)
	}
	for model, name := range map[any]string{
		&models.Budget{}: "budgets", &models.Receivable{}: "receivables", &models.ReceivableSettlement{}: "settlements",
		&models.TransactionRule{}: "rules", &models.FixedDepositDetails{}: "fixed deposits",
	} {
		if n := countRows(t, db, model, "1 = 1"); n == 0 {
			t.Errorf("other user's %s were removed", name)
		}
	}
	if n := countRows(t, db, &models.DemoRow{}, "user_id = ?", user.ID); n != 0 {
		t.Errorf("%d demo rows remain", n)
	}
	var kept models.Account
	db.First(&kept, "id = ?", realAcct.ID)
	if kept.Balance != 5000 {
		t.Errorf("real balance changed to %d", kept.Balance)
	}

	// The other user's household is intact and the shared securities survive.
	if n := countRows(t, db, &models.Account{}, "user_id = ?", other.ID); n < 5 {
		t.Errorf("other user lost accounts: %d", n)
	}
	if n := countRows(t, db, &models.Security{}, "1 = 1"); int(n) != len(demoHoldings) {
		t.Errorf("shared securities = %d", n)
	}
	if n := countRows(t, db, &models.SecurityPrice{}, "1 = 1"); n == 0 {
		t.Error("shared prices were deleted")
	}

	// Removing again is a no-op.
	res, err = svc.Remove(user.ID)
	testutil.AssertNoError(t, err)
	if res.Total != 0 {
		t.Errorf("second removal touched %d rows", res.Total)
	}

	// The last user to remove it takes the securities with it.
	_, err = svc.Remove(other.ID)
	testutil.AssertNoError(t, err)
	if n := countRows(t, db, &models.Security{}, "1 = 1"); n != 0 {
		t.Errorf("%d securities left", n)
	}
	if n := countRows(t, db, &models.SecurityPrice{}, "1 = 1"); n != 0 {
		t.Errorf("%d prices left", n)
	}
}

func TestDemoRemoveTakesEntriesAddedInsideSampleAccounts(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)
	realAcct := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 7000)
	_, err := svc.Load(user.ID)
	testutil.AssertNoError(t, err)

	var dbs, hys, ibkr models.Account
	db.Where("user_id = ? AND name = ?", user.ID, "DBS Everyday").First(&dbs)
	db.Where("user_id = ? AND name = ?", user.ID, "High-Yield Savings").First(&hys)
	db.Where("user_id = ? AND name = ?", user.ID, "IBKR Brokerage").First(&ibkr)
	mine := &models.Transaction{UserID: user.ID, AccountID: dbs.ID, Type: models.TransactionTypeExpense, Amount: 900, Description: "mine", Date: time.Now()}
	db.Create(mine)
	db.Create(&models.Transaction{UserID: user.ID, AccountID: dbs.ID, ToAccountID: &hys.ID, Type: models.TransactionTypeTransfer, Amount: 100, Description: "between samples", Date: time.Now()})
	var groceries models.Category
	db.Where("user_id = ? AND name = ?", user.ID, "Groceries").First(&groceries)
	db.Create(&models.Budget{UserID: user.ID, CategoryID: groceries.ID, Name: "mine", Amount: 5000, Period: models.BudgetPeriodYearly, IsActive: true, Currency: "MYR"})
	sec := testutil.CreateTestSecurityWithParams(t, db, "NEW", "New Co", models.AssetTypeStock, "NASDAQ")
	testutil.CreateTestInvestment(t, db, ibkr.ID, sec.ID)
	// A real entry on a real account stays, even when it uses a sample category.
	realTx := &models.Transaction{UserID: user.ID, AccountID: realAcct.ID, CategoryID: &groceries.ID, Type: models.TransactionTypeExpense, Amount: 50, Date: time.Now()}
	db.Create(realTx)

	_, err = svc.Remove(user.ID)
	testutil.AssertNoError(t, err)
	if n := countRows(t, db, &models.Transaction{}, "user_id = ?", user.ID); n != 1 {
		t.Errorf("only the real-account transaction should remain, %d do", n)
	}
	if n := countRows(t, db, &models.Account{}, "user_id = ?", user.ID); n != 1 {
		t.Errorf("accounts left: %d", n)
	}
	if n := countRows(t, db, &models.Budget{}, "user_id = ?", user.ID); n != 0 {
		t.Errorf("budgets left: %d", n)
	}
	if n := countRows(t, db, &models.Investment{}, "1 = 1"); n != 0 {
		t.Errorf("investments left: %d", n)
	}
	var kept models.Transaction
	db.First(&kept, "id = ?", realTx.ID)
	if kept.CategoryID != nil {
		t.Error("the sample category should have been detached from the real transaction")
	}
}

func TestDemoRemoveRefusesWhenItWouldDamageOtherData(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)
	realAcct := testutil.CreateTestCashAccount(t, db, user.ID)
	_, err := svc.Load(user.ID)
	testutil.AssertNoError(t, err)
	var dbs models.Account
	db.Where("user_id = ? AND name = ?", user.ID, "DBS Everyday").First(&dbs)

	// A receipt on a sample transaction: the file lives in object storage.
	var sample models.Transaction
	db.Where("user_id = ? AND account_id = ? AND type = ?", user.ID, dbs.ID, models.TransactionTypeExpense).First(&sample)
	att := &models.TransactionAttachment{UserID: user.ID, TransactionID: sample.ID, StorageKey: "k", ContentType: "image/png", ByteSize: 1, ChecksumSHA256: "x"}
	db.Create(att)
	_, err = svc.Remove(user.ID)
	testutil.AssertAppError(t, err, "DEMO_IN_USE")
	db.Unscoped().Delete(att)

	// A transfer with one real leg (the API blocks these, so it was written some other way).
	db.Create(&models.Transaction{UserID: user.ID, AccountID: dbs.ID, ToAccountID: &realAcct.ID, Type: models.TransactionTypeTransfer, Amount: 1, Date: time.Now()})
	_, err = svc.Remove(user.ID)
	testutil.AssertAppError(t, err, "DEMO_IN_USE")

	if n := countRows(t, db, &models.Account{}, "user_id = ?", user.ID); n < 5 {
		t.Errorf("a refused removal must change nothing, accounts: %d", n)
	}
	if n := countRows(t, db, &models.DemoRow{}, "user_id = ?", user.ID); n == 0 {
		t.Error("demo rows were dropped by a refused removal")
	}
}

func TestGuardDemoTransfer(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)
	realAcct := testutil.CreateTestCashAccount(t, db, user.ID)
	_, err := svc.Load(user.ID)
	testutil.AssertNoError(t, err)
	var dbs, hys models.Account
	db.Where("user_id = ? AND name = ?", user.ID, "DBS Everyday").First(&dbs)
	db.Where("user_id = ? AND name = ?", user.ID, "High-Yield Savings").First(&hys)

	testutil.AssertAppError(t, guardDemoTransfer(db, user.ID, dbs.ID, realAcct.ID), "DEMO_TRANSFER_BLOCKED")
	testutil.AssertAppError(t, guardDemoTransfer(db, user.ID, realAcct.ID, hys.ID), "DEMO_TRANSFER_BLOCKED")
	testutil.AssertNoError(t, guardDemoTransfer(db, user.ID, dbs.ID, hys.ID))
	real2 := testutil.CreateTestCashAccount(t, db, user.ID)
	testutil.AssertNoError(t, guardDemoTransfer(db, user.ID, realAcct.ID, real2.ID))
}

func TestDemoRemoveClearsAccountOrphans(t *testing.T) {
	svc, db := newDemoTestService(t)
	user := testutil.CreateTestUser(t, db)
	if _, err := svc.Load(user.ID); err != nil {
		t.Fatalf("load: %v", err)
	}
	var acct models.Account
	if err := db.Where("user_id = ?", user.ID).First(&acct).Error; err != nil {
		t.Fatalf("sample account: %v", err)
	}
	day := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&models.Reminder{UserID: user.ID, Kind: models.ReminderKindCardDue, RefID: acct.ID,
		DueOn: day.AddDate(0, 1, 0), RemindAt: day}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AccountBalanceSnapshot{AccountID: acct.ID, Day: day, UserID: user.ID,
		Balance: 100, Currency: acct.Currency}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Remove(user.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n := countRows(t, db, &models.Reminder{}, "user_id = ?", user.ID); n != 0 {
		t.Errorf("reminders left for removed sample accounts: %d", n)
	}
	if n := countRows(t, db, &models.AccountBalanceSnapshot{}, "user_id = ?", user.ID); n != 0 {
		t.Errorf("balance snapshots left for removed sample accounts: %d", n)
	}
}
