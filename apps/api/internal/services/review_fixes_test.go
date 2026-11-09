package services

import (
	"strings"
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"

	"gorm.io/gorm"
)

func relabelAccount(t *testing.T, db *gorm.DB, a *models.Account, currency string) {
	t.Helper()
	testutil.AssertNoError(t, db.Model(a).Update("currency", currency).Error)
	a.Currency = currency
}

func TestUpdateTransaction_RejectsMoveAcrossCurrencies(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)
	usd := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	myr := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	relabelAccount(t, db, myr, "MYR")
	same := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	relabelAccount(t, db, same, "usd") // case-insensitive match

	tx, err := txSvc.CreateTransaction(user.ID, usd.ID, nil, models.TransactionTypeExpense, 5000, "x", time.Now())
	testutil.AssertNoError(t, err)

	_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{AccountID: &myr.ID})
	testutil.AssertAppError(t, err, apperrors.ErrAccountCurrencyMismatch.Code)
	if got := balanceOf(t, db, usd.ID); got != 95000 {
		t.Errorf("source balance = %d, want 95000 (unchanged)", got)
	}
	if got := balanceOf(t, db, myr.ID); got != 100000 {
		t.Errorf("target balance = %d, want 100000 (unchanged)", got)
	}

	_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{AccountID: &same.ID})
	testutil.AssertNoError(t, err)
}

func TestCloseFixedDeposit_RejectsDifferentCurrencyDestination(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	user := testutil.CreateTestUser(t, db)
	fd, _ := testutil.CreateTestFixedDepositAccount(t, db, user.ID, testutil.FixedDepositParams{}) // USD
	dest := testutil.CreateTestCashAccount(t, db, user.ID)
	relabelAccount(t, db, dest, "MYR")
	zero := int64(0)

	_, err := acctSvc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: &dest.ID, InterestOverride: &zero})
	testutil.AssertAppError(t, err, apperrors.ErrAccountCurrencyMismatch.Code)
	if got := balanceOf(t, db, dest.ID); got != 0 {
		t.Errorf("destination balance = %d, want 0", got)
	}
	if got := balanceOf(t, db, fd.ID); got != 100000 {
		t.Errorf("fd balance = %d, want 100000", got)
	}

	ok := testutil.CreateTestCashAccount(t, db, user.ID) // USD
	_, err = acctSvc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: &ok.ID, InterestOverride: &zero})
	testutil.AssertNoError(t, err)
}

func TestCreate_RejectsNonTwoDecimalCurrencies(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	secSvc := NewSecurityService(db)
	budSvc := NewBudgetService(db)
	user := testutil.CreateTestUser(t, db)
	want := apperrors.ErrUnsupportedCurrency.Code

	_, err := acctSvc.CreateCashAccount(user.ID, "a", "", "JPY", 0)
	testutil.AssertAppError(t, err, want)
	_, err = acctSvc.CreateCashAccount(user.ID, "a", "", "kwd", 0)
	testutil.AssertAppError(t, err, want)
	_, err = acctSvc.CreateInvestmentAccount(user.ID, "a", "", "JPY", "", "")
	testutil.AssertAppError(t, err, want)
	_, err = acctSvc.CreateCreditCardAccount(user.ID, "a", "", "JPY", 100, 1, nil, 0)
	testutil.AssertAppError(t, err, want)

	src := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1_000_000)
	relabelAccount(t, db, src, "JPY")
	_, err = acctSvc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
		Name: "fd", Currency: "JPY", SourceAccountID: src.ID, Principal: 1000, InterestRate: 1,
		StartDate: time.Now(), MaturityDate: time.Now().AddDate(1, 0, 0), PayoutType: models.PayoutTypeCumulative,
	})
	testutil.AssertAppError(t, err, want)

	_, err = secSvc.CreateSecurity("JPYS", "Yen sec", models.AssetTypeStock, "JPY", "TSE", nil)
	testutil.AssertAppError(t, err, want)
	sec, err := secSvc.CreateSecurity("OKS", "Ok sec", models.AssetTypeStock, "EUR", "XETRA", nil)
	testutil.AssertNoError(t, err)
	_, err = secSvc.UpdateSecurity(sec.ID, SecurityUpdate{Currency: strPtr("KWD")})
	testutil.AssertAppError(t, err, want)

	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	_, err = budSvc.CreateBudget(user.ID, cat.ID, "b", 1000, models.BudgetPeriodMonthly, WithBudgetCurrency("JPY"))
	testutil.AssertAppError(t, err, want)
	b, err := budSvc.CreateBudget(user.ID, cat.ID, "b", 1000, models.BudgetPeriodMonthly, WithBudgetCurrency("EUR"))
	testutil.AssertNoError(t, err)
	_, err = budSvc.UpdateBudget(user.ID, b.ID, "b", nil, nil, nil, WithBudgetCurrency("JPY"))
	testutil.AssertAppError(t, err, want)
}

func TestIsTwoDecimalCurrency(t *testing.T) {
	for code, want := range map[string]bool{"USD": true, "MYR": true, "JPY": false, "KWD": false} {
		if got := isTwoDecimalCurrency(code); got != want {
			t.Errorf("isTwoDecimalCurrency(%s) = %v, want %v", code, got, want)
		}
	}
}

func TestImportedTransactionsAreLocked(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)

	var row models.Transaction
	testutil.AssertNoError(t, e.db.Where("account_id = ? AND type = ? AND import_ref LIKE ?",
		e.cash.ID, models.TransactionTypeExpense, "%:dr").First(&row).Error)

	income := models.TransactionTypeIncome
	_, err = e.txSvc.UpdateTransaction(e.userID, row.ID, TransactionUpdateFields{Type: &income})
	testutil.AssertAppError(t, err, apperrors.ErrImportedTransactionLocked.Code)
	amt := row.Amount + 1
	_, err = e.txSvc.UpdateTransaction(e.userID, row.ID, TransactionUpdateFields{Amount: &amt})
	testutil.AssertAppError(t, err, apperrors.ErrImportedTransactionLocked.Code)
	other := e.bank.ID
	_, err = e.txSvc.UpdateTransaction(e.userID, row.ID, TransactionUpdateFields{AccountID: &other})
	testutil.AssertAppError(t, err, apperrors.ErrImportedTransactionLocked.Code)

	desc := "my note"
	_, err = e.txSvc.UpdateTransaction(e.userID, row.ID, TransactionUpdateFields{Description: &desc})
	testutil.AssertNoError(t, err)
}

func TestImportIBKR_ReimportReversesByCurrentType(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	want := balanceOf(t, e.db, e.cash.ID)

	// Simulate a legacy row whose type was flipped by an edit: the balance
	// moved with it (reverse the expense, apply the income).
	var row models.Transaction
	testutil.AssertNoError(t, e.db.Where("account_id = ? AND type = ? AND import_ref LIKE ?",
		e.cash.ID, models.TransactionTypeExpense, "%:dr").First(&row).Error)
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Where("id = ?", row.ID).
		Update("type", models.TransactionTypeIncome).Error)
	testutil.AssertNoError(t, e.db.Model(&models.Account{}).Where("id = ?", e.cash.ID).
		Update("balance", gorm.Expr("balance + ?", 2*row.Amount)).Error)

	_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if got := balanceOf(t, e.db, e.cash.ID); got != want {
		t.Errorf("cash balance after re-import = %d, want %d", got, want)
	}
}

func TestUpdateTransaction_ImportRefTypeFlipDoesNotCorrupt(t *testing.T) {
	// reversalFor unit check: the current type wins over the ref direction.
	ref := "ibkr:U1:abc:dr"
	inc := &models.Transaction{Type: models.TransactionTypeIncome, ImportRef: &ref}
	if rev, ok := reversalFor(inc); !ok || rev != models.TransactionTypeExpense {
		t.Errorf("income row reversal = %v %v, want expense", rev, ok)
	}
	exp := &models.Transaction{Type: models.TransactionTypeExpense, ImportRef: &ref}
	if rev, ok := reversalFor(exp); !ok || rev != models.TransactionTypeIncome {
		t.Errorf("expense row reversal = %v %v, want income", rev, ok)
	}
}

func TestRecordSell_FloatDustFullSell(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []float64 // first is the buy, rest are sells
		last  float64
	}{
		{"0.3 buy, sell 0.1 then 0.2", []float64{0.3, 0.1}, 0.2},
		{"0.7 + 0.1 buy, sell 0.8", []float64{0.7, 0.1}, 0.8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			acctSvc := NewAccountService(db)
			svc := NewInvestmentService(db, acctSvc, nil)
			user := testutil.CreateTestUser(t, db)
			account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
			sec := testutil.CreateTestSecurity(t, db)
			inv := &models.Investment{AccountID: account.ID, SecurityID: sec.ID}
			testutil.AssertNoError(t, db.Create(inv).Error)

			now := time.Now()
			if tc.name[0:3] == "0.3" {
				_, err := svc.RecordBuy(user.ID, inv.ID, BuyParams{Date: now, Quantity: 0.3, PricePerUnit: 10000})
				testutil.AssertNoError(t, err)
				_, err = svc.RecordSell(user.ID, inv.ID, SellParams{Date: now.Add(time.Second), Quantity: 0.1, PricePerUnit: 10000})
				testutil.AssertNoError(t, err)
			} else {
				_, err := svc.RecordBuy(user.ID, inv.ID, BuyParams{Date: now, Quantity: 0.7, PricePerUnit: 10000})
				testutil.AssertNoError(t, err)
				_, err = svc.RecordBuy(user.ID, inv.ID, BuyParams{Date: now.Add(time.Second), Quantity: 0.1, PricePerUnit: 10000})
				testutil.AssertNoError(t, err)
			}
			_, err := svc.RecordSell(user.ID, inv.ID, SellParams{Date: now.Add(2 * time.Second), Quantity: tc.last, PricePerUnit: 10000})
			testutil.AssertNoError(t, err)

			var got models.Investment
			testutil.AssertNoError(t, db.First(&got, "id = ?", inv.ID).Error)
			if got.Quantity != 0 || got.CostBasis != 0 {
				t.Errorf("holding = qty %v cost %d, want fully closed", got.Quantity, got.CostBasis)
			}
		})
	}
}

func TestInvestmentAccountBalance_FlagsHoldingWithoutRate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	fxs := NewFXService(db, testFXStaleness)
	acctSvc := NewAccountService(db, WithFXService(fxs))
	user := testutil.CreateTestUser(t, db)                          // base MYR
	account := testutil.CreateTestInvestmentAccount(t, db, user.ID) // MYR, so base conversion needs no rate
	usd := testutil.CreateTestSecurity(t, db)
	testutil.AssertNoError(t, db.Model(usd).Update("currency", "USD").Error)
	testutil.AssertNoError(t, db.Create(&models.SecurityPrice{SecurityID: usd.ID, Price: 1000, Currency: "USD", RecordedAt: time.Now()}).Error)
	testutil.AssertNoError(t, db.Create(&models.Investment{AccountID: account.ID, SecurityID: usd.ID, Quantity: 2}).Error)

	got, err := acctSvc.GetAccountByID(user.ID, account.ID)
	testutil.AssertNoError(t, err)
	if !got.FXMissing {
		t.Error("GetAccountByID: FXMissing = false, want true (USD holding has no rate to MYR)")
	}
	if got.Balance != 0 {
		t.Errorf("balance = %d, want 0 (holding left out)", got.Balance)
	}

	page, err := acctSvc.GetUserAccounts(user.ID, pagination.PageRequest{Page: 1, PageSize: 20})
	testutil.AssertNoError(t, err)
	found := false
	for _, a := range page.Data {
		if a.ID == account.ID {
			found = true
			if !a.FXMissing {
				t.Error("GetUserAccounts: FXMissing = false, want true")
			}
		}
	}
	if !found {
		t.Fatal("account missing from list")
	}
}

func TestRecordPrices_CurrencyRequiredPerEntry(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	sec := testutil.CreateTestSecurity(t, db)
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	res, err := NewSecurityService(db).RecordPrices([]SecurityPriceInput{
		{SecurityID: sec.ID, Price: 1, RecordedAt: at},
		{SecurityID: sec.ID, Price: 2, Currency: "MYR", RecordedAt: at.Add(time.Hour)},
	})
	testutil.AssertNoError(t, err)
	if res.Recorded != 1 || len(res.Rejected) != 1 || res.Rejected[0].Reason != PriceRejectedCurrencyRequired || res.Rejected[0].Index != 0 {
		t.Fatalf("result = %+v", res)
	}
	if !strings.EqualFold(PriceRejectedCurrencyRequired, "currency_required") {
		t.Error("unexpected reason code")
	}
}
