package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// Investment balances are derived from holdings and a fixed deposit's principal
// is locked, so plain income, expense and transfer rows must not target them.
func TestPlainActivityRefusedOnInvestmentAndFixedDeposit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)

	cash := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 500_000)
	invest := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	// The internal path (deposit creation) funds the deposit from cash.
	fd, err := acctSvc.CreateFixedDepositAccount(user.ID, CreateFixedDepositParams{
		Name:            "FD",
		Currency:        cash.Currency,
		SourceAccountID: cash.ID,
		Principal:       100_000,
		InterestRate:    4,
		StartDate:       time.Now().AddDate(0, -1, 0),
		MaturityDate:    time.Now().AddDate(1, 0, 0),
		PayoutType:      models.PayoutTypeCumulative,
		Compounding:     models.CompoundingAnnual,
	})
	testutil.AssertNoError(t, err)

	for _, target := range []*models.Account{invest, fd} {
		for _, typ := range []models.TransactionType{models.TransactionTypeExpense, models.TransactionTypeIncome} {
			_, err := txSvc.CreateTransaction(user.ID, target.ID, nil, typ, 1_000, "nope", time.Now(), TransactionMeta{})
			testutil.AssertAppError(t, err, "ACCOUNT_TYPE_NOT_ALLOWED")
		}
		_, err := txSvc.CreateTransfer(user.ID, cash.ID, target.ID, 1_000, nil, "nope", time.Now(), TransactionMeta{})
		testutil.AssertAppError(t, err, "ACCOUNT_TYPE_NOT_ALLOWED")
		_, err = txSvc.CreateTransfer(user.ID, target.ID, cash.ID, 1_000, nil, "nope", time.Now(), TransactionMeta{})
		testutil.AssertAppError(t, err, "ACCOUNT_TYPE_NOT_ALLOWED")
	}

	// Moving an existing expense onto one of them is refused too.
	moved, err := txSvc.CreateTransaction(user.ID, cash.ID, nil, models.TransactionTypeExpense, 2_000, "lunch", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	for _, target := range []*models.Account{invest, fd} {
		to := target.ID
		_, err := txSvc.UpdateTransaction(user.ID, moved.ID, TransactionUpdateFields{AccountID: &to})
		testutil.AssertAppError(t, err, "ACCOUNT_TYPE_NOT_ALLOWED")
	}

	// Nothing moved: cash paid the deposit and the lunch only.
	got, err := acctSvc.GetAccountByID(user.ID, cash.ID)
	testutil.AssertNoError(t, err)
	if got.Balance != 500_000-100_000-2_000 {
		t.Errorf("cash balance = %d, want %d", got.Balance, 500_000-100_000-2_000)
	}
	gotFD, err := acctSvc.GetAccountByID(user.ID, fd.ID)
	testutil.AssertNoError(t, err)
	if gotFD.Balance != 100_000 {
		t.Errorf("deposit balance = %d, want 100000", gotFD.Balance)
	}

	// Cash accounts, cards and loans still take plain activity, and the
	// internal close path (which writes its own rows) still works.
	_, err = txSvc.CreateTransaction(user.ID, cash.ID, nil, models.TransactionTypeIncome, 500, "ok", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	_, err = acctSvc.CloseFixedDeposit(user.ID, fd.ID, CloseFixedDepositParams{DestinationAccountID: &cash.ID})
	testutil.AssertNoError(t, err)
}
