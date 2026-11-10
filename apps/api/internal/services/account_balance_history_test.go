package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// abhTx inserts a transaction on a UTC day offset from today (negative = past).
func abhTx(t *testing.T, db *gorm.DB, userID, accountID string, typ models.TransactionType, amount int64, daysAgo int, to *string, toAmount *int64) {
	t.Helper()
	day := utcDay(time.Now()).AddDate(0, 0, -daysAgo).Add(10 * time.Hour)
	testutil.AssertNoError(t, db.Create(&models.Transaction{
		UserID: userID, AccountID: accountID, Type: typ, Amount: amount, Date: day,
		ToAccountID: to, ToAmount: toAmount,
	}).Error)
}

func abhRows(t *testing.T, db *gorm.DB, accountID string) map[string]int64 {
	t.Helper()
	var rows []models.AccountBalanceSnapshot
	testutil.AssertNoError(t, db.Where("account_id = ?", accountID).Order("day ASC").Find(&rows).Error)
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Day.UTC().Format(dayLayout)] = r.Balance
	}
	return out
}

func abhDay(daysAgo int) string {
	return utcDay(time.Now()).AddDate(0, 0, -daysAgo).Format(dayLayout)
}

func TestRecordAccountBalances_BackfillsCashAndWritesToday(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	a := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 8500)
	abhTx(t, db, u.ID, a.ID, models.TransactionTypeOpeningBalance, 10000, 4, nil, nil)
	abhTx(t, db, u.ID, a.ID, models.TransactionTypeExpense, 2000, 2, nil, nil)
	abhTx(t, db, u.ID, a.ID, models.TransactionTypeIncome, 500, 1, nil, nil)

	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))

	got := abhRows(t, db, a.ID)
	want := map[string]int64{
		abhDay(4): 10000, abhDay(3): 10000, abhDay(2): 8000, abhDay(1): 8500, abhDay(0): 8500,
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for d, v := range want {
		if got[d] != v {
			t.Errorf("day %s = %d, want %d (all: %v)", d, got[d], v, got)
		}
	}

	// Idempotent: a re-run keeps one row per day, overwrites today and leaves
	// the backfilled days alone even though the account now has history.
	testutil.AssertNoError(t, db.Model(a).Update("balance", 9000).Error)
	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))
	got = abhRows(t, db, a.ID)
	if len(got) != 5 || got[abhDay(0)] != 9000 || got[abhDay(2)] != 8000 {
		t.Errorf("after re-run = %v", got)
	}
}

func TestRecordAccountBalances_CreditCardInvertsAndTransfersUseBothLegs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	card := testutil.CreateTestCreditCardAccount(t, db, u.ID, 0)
	cash := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 0)
	other := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 0)
	// Card: owes 3000 (opening, never inverted), spends 1000 more, pays back 1500 from cash.
	abhTx(t, db, u.ID, card.ID, models.TransactionTypeOpeningBalance, 3000, 5, nil, nil)
	abhTx(t, db, u.ID, card.ID, models.TransactionTypeExpense, 1000, 3, nil, nil)
	// Cash: 5000 opening, pays the card 1500, sends 400 to another account that receives 90 (to_amount).
	abhTx(t, db, u.ID, cash.ID, models.TransactionTypeOpeningBalance, 5000, 5, nil, nil)
	abhTx(t, db, u.ID, cash.ID, models.TransactionTypeTransfer, 1500, 2, &card.ID, nil)
	toAmt := int64(90)
	abhTx(t, db, u.ID, cash.ID, models.TransactionTypeTransfer, 400, 1, &other.ID, &toAmt)
	testutil.AssertNoError(t, db.Model(card).Update("balance", 2500).Error) // 3000 + 1000 - 1500
	testutil.AssertNoError(t, db.Model(cash).Update("balance", 3100).Error)
	testutil.AssertNoError(t, db.Model(other).Update("balance", 90).Error)

	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))

	cardRows := abhRows(t, db, card.ID)
	if cardRows[abhDay(5)] != 3000 || cardRows[abhDay(3)] != 4000 || cardRows[abhDay(2)] != 2500 || cardRows[abhDay(0)] != 2500 {
		t.Errorf("card = %v", cardRows)
	}
	cashRows := abhRows(t, db, cash.ID)
	if cashRows[abhDay(2)] != 3500 || cashRows[abhDay(1)] != 3100 {
		t.Errorf("cash = %v", cashRows)
	}
	otherRows := abhRows(t, db, other.ID)
	if len(otherRows) != 2 || otherRows[abhDay(1)] != 90 || otherRows[abhDay(0)] != 90 {
		t.Errorf("other = %v", otherRows)
	}
}

func TestRecordAccountBalances_NotReplayableAccountsOnlyGetToday(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")

	withInvestmentRow := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 700)
	abhTx(t, db, u.ID, withInvestmentRow.ID, models.TransactionTypeOpeningBalance, 1000, 3, nil, nil)
	abhTx(t, db, u.ID, withInvestmentRow.ID, models.TransactionTypeInvestment, 300, 2, nil, nil)

	drifted := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 999) // replay says 1000
	abhTx(t, db, u.ID, drifted.ID, models.TransactionTypeOpeningBalance, 1000, 3, nil, nil)

	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))

	for name, a := range map[string]*models.Account{"investment row": withInvestmentRow, "drift": drifted} {
		got := abhRows(t, db, a.ID)
		if len(got) != 1 || got[abhDay(0)] != a.Balance {
			t.Errorf("%s: rows = %v, want only today = %d", name, got, a.Balance)
		}
	}
}

func TestRecordAccountBalances_SkipsPastRecordedAtAndInactive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	a := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 100)
	inactive := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 100)
	testutil.AssertNoError(t, db.Model(inactive).Update("is_active", false).Error)

	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now().AddDate(0, 0, -10)))
	if got := abhRows(t, db, a.ID); len(got) != 0 {
		t.Errorf("a past recordedAt wrote rows: %v", got)
	}
	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))
	if len(abhRows(t, db, a.ID)) != 1 || len(abhRows(t, db, inactive.ID)) != 0 {
		t.Errorf("active/inactive rows = %v / %v", abhRows(t, db, a.ID), abhRows(t, db, inactive.ID))
	}
}

func TestRecordAccountBalances_InvestmentAccountIsMarketValue(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	inv := testutil.CreateTestInvestmentAccount(t, db, u.ID)
	sec := testutil.CreateTestSecurity(t, db)
	h := testutil.CreateTestInvestment(t, db, inv.ID, sec.ID) // quantity 10
	testutil.AssertNoError(t, db.Model(&models.Investment{}).Where("id = ?", h.ID).Update("quantity", 4).Error)
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 250, time.Now().Add(-time.Hour))
	empty := testutil.CreateTestInvestmentAccount(t, db, u.ID)

	testutil.AssertNoError(t, db.Model(inv).Update("currency", "USD").Error)
	testutil.AssertNoError(t, db.Model(empty).Update("currency", "USD").Error)
	testutil.AssertNoError(t, db.Model(&models.Security{}).Where("id = ?", sec.ID).Update("currency", "USD").Error)
	testutil.AssertNoError(t, recordAccountBalances(db, nil, time.Now()))

	if got := abhRows(t, db, inv.ID); len(got) != 1 || got[abhDay(0)] != 1000 {
		t.Errorf("investment rows = %v, want today = 1000", got)
	}
	if got := abhRows(t, db, empty.ID); len(got) != 1 || got[abhDay(0)] != 0 {
		t.Errorf("empty investment rows = %v, want today = 0", got)
	}
}

func TestComputeAndRecordSnapshots_AlsoRecordsAccountBalances(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "USD")
	a := testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 4200)
	_, err := NewPortfolioSnapshotService(db, nil).ComputeAndRecordSnapshots(time.Now())
	testutil.AssertNoError(t, err)
	if got := abhRows(t, db, a.ID); got[abhDay(0)] != 4200 {
		t.Errorf("rows = %v", got)
	}
}

func TestAccountBalanceHistory_ScopingRangeAndSparklines(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewAccountBalanceHistoryService(db)
	userA := p4User(t, db, "USD")
	userB := testutil.CreateTestUserWithEmail(t, db, "b@example.com")
	a := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 300)
	b := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 77)
	for i, bal := range []int64{100, 200, 300} {
		testutil.AssertNoError(t, db.Create(&models.AccountBalanceSnapshot{
			AccountID: a.ID, UserID: userA.ID, Day: utcDay(time.Now()).AddDate(0, 0, i-2), Balance: bal, Currency: "USD",
		}).Error)
	}
	testutil.AssertNoError(t, db.Create(&models.AccountBalanceSnapshot{
		AccountID: b.ID, UserID: userB.ID, Day: utcDay(time.Now()), Balance: 77, Currency: "USD",
	}).Error)

	h, err := svc.GetBalanceHistory(userA.ID, a.ID, time.Time{}, time.Time{})
	testutil.AssertNoError(t, err)
	if len(h.Points) != 3 || h.Points[0].Balance != 100 || h.Points[2].Balance != 300 || h.Currency != "USD" {
		t.Errorf("history = %+v", h)
	}
	h, err = svc.GetBalanceHistory(userA.ID, a.ID, utcDay(time.Now()).AddDate(0, 0, -1), time.Now())
	testutil.AssertNoError(t, err)
	if len(h.Points) != 2 {
		t.Errorf("ranged history = %+v", h.Points)
	}

	// User B's account is invisible to user A.
	_, err = svc.GetBalanceHistory(userA.ID, b.ID, time.Time{}, time.Time{})
	testutil.AssertAppError(t, err, apperrors.ErrAccountNotFound.Code)
	// Range validation.
	_, err = svc.GetBalanceHistory(userA.ID, a.ID, time.Now(), time.Now().AddDate(0, 0, -3))
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.GetBalanceHistory(userA.ID, a.ID, time.Now().AddDate(-20, 0, 0), time.Now())
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)

	spark, err := svc.GetSparklines(userA.ID, 2)
	testutil.AssertNoError(t, err)
	if len(spark) != 1 || spark[0].AccountID != a.ID || len(spark[0].Points) != 2 || spark[0].Points[1].Balance != 300 {
		t.Errorf("sparklines = %+v", spark)
	}
	_, err = svc.GetSparklines(userA.ID, 1)
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	_, err = svc.GetSparklines(userA.ID, 400)
	testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	if def, err := svc.GetSparklines(userA.ID, 0); err != nil || len(def) != 1 || len(def[0].Points) != 3 {
		t.Errorf("default days = %+v, %v", def, err)
	}
}
