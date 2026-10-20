package services

import (
	"testing"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestApplyBalanceDelta(t *testing.T) {
	const amount = 2500
	tests := []struct {
		name     string
		acctType models.AccountType
		txnType  models.TransactionType
		startBal int64
		wantBal  int64
	}{
		{"cash_income", models.AccountTypeCash, models.TransactionTypeIncome, 10000, 12500},
		{"cash_expense", models.AccountTypeCash, models.TransactionTypeExpense, 10000, 7500},
		{"cash_reimbursement", models.AccountTypeCash, models.TransactionTypeReimbursement, 10000, 12500},
		{"credit_card_income_reduces_owed", models.AccountTypeCreditCard, models.TransactionTypeIncome, 10000, 7500},
		{"credit_card_expense_increases_owed", models.AccountTypeCreditCard, models.TransactionTypeExpense, 10000, 12500},
		{"credit_card_reimbursement_reduces_owed", models.AccountTypeCreditCard, models.TransactionTypeReimbursement, 10000, 7500},
		{"debt_income_reduces_owed", models.AccountTypeDebt, models.TransactionTypeIncome, 10000, 7500},
		{"debt_expense_increases_owed", models.AccountTypeDebt, models.TransactionTypeExpense, 10000, 12500},
		{"debt_reimbursement_reduces_owed", models.AccountTypeDebt, models.TransactionTypeReimbursement, 10000, 7500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			svc := NewAccountService(db)
			user := testutil.CreateTestUser(t, db)

			var account *models.Account
			switch tt.acctType {
			case models.AccountTypeCreditCard:
				account = testutil.CreateTestCreditCardAccount(t, db, user.ID, tt.startBal)
			case models.AccountTypeDebt:
				account = testutil.CreateTestDebtAccount(t, db, user.ID, tt.startBal)
			default:
				account = testutil.CreateTestCashAccountWithBalance(t, db, user.ID, tt.startBal)
			}

			err := svc.ApplyBalanceDelta(db, account.ID, tt.acctType, tt.txnType, amount)
			testutil.AssertNoError(t, err)

			got, err := svc.GetAccountByID(user.ID, account.ID)
			testutil.AssertNoError(t, err)
			if got.Balance != tt.wantBal {
				t.Errorf("balance = %d, want %d", got.Balance, tt.wantBal)
			}
		})
	}

	t.Run("is_relative_not_absolute", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		// Another writer changes the balance after we loaded our copy.
		testutil.AssertNoError(t, db.Model(&models.Account{}).Where("id = ?", account.ID).UpdateColumn("balance", 5000).Error)

		testutil.AssertNoError(t, svc.ApplyBalanceDelta(db, account.ID, models.AccountTypeCash, models.TransactionTypeIncome, 100))

		got, err := svc.GetAccountByID(user.ID, account.ID)
		testutil.AssertNoError(t, err)
		if got.Balance != 5100 {
			t.Errorf("balance = %d, want 5100 (stale copy must not overwrite)", got.Balance)
		}
	})

	t.Run("rejects_unsupported_type", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		err := svc.ApplyBalanceDelta(db, account.ID, models.AccountTypeCash, models.TransactionTypeTransfer, 100)
		testutil.AssertAppError(t, err, apperrors.ErrInvalidTransactionType.Code)
	})

	t.Run("rejects_negative_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		err := svc.ApplyBalanceDelta(db, account.ID, models.AccountTypeCash, models.TransactionTypeIncome, -1)
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})

	t.Run("unknown_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)

		err := svc.ApplyBalanceDelta(db, "00000000-0000-0000-0000-00000000dead", models.AccountTypeCash, models.TransactionTypeIncome, 100)
		testutil.AssertAppError(t, err, apperrors.ErrAccountNotFound.Code)
	})
}

func TestUpdateAccountBalance_DebtAndReimbursement(t *testing.T) {
	tests := []struct {
		name     string
		acctType models.AccountType
		txnType  models.TransactionType
		want     int64
	}{
		{"debt_expense_increases_owed", models.AccountTypeDebt, models.TransactionTypeExpense, 10500},
		{"debt_income_decreases_owed", models.AccountTypeDebt, models.TransactionTypeIncome, 9500},
		{"debt_reimbursement_decreases_owed", models.AccountTypeDebt, models.TransactionTypeReimbursement, 9500},
		{"credit_card_expense_increases_owed", models.AccountTypeCreditCard, models.TransactionTypeExpense, 10500},
		{"credit_card_reimbursement_decreases_owed", models.AccountTypeCreditCard, models.TransactionTypeReimbursement, 9500},
		{"cash_reimbursement_credits", models.AccountTypeCash, models.TransactionTypeReimbursement, 10500},
		{"cash_expense_debits", models.AccountTypeCash, models.TransactionTypeExpense, 9500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			defer testutil.TeardownTestDB(t, db)
			svc := NewAccountService(db)
			user := testutil.CreateTestUser(t, db)

			var account *models.Account
			switch tt.acctType {
			case models.AccountTypeCreditCard:
				account = testutil.CreateTestCreditCardAccount(t, db, user.ID, 10000)
			case models.AccountTypeDebt:
				account = testutil.CreateTestDebtAccount(t, db, user.ID, 10000)
			default:
				account = testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
			}

			testutil.AssertNoError(t, svc.UpdateAccountBalance(db, account, tt.txnType, 500))

			got, err := svc.GetAccountByID(user.ID, account.ID)
			testutil.AssertNoError(t, err)
			if got.Balance != tt.want {
				t.Errorf("balance = %d, want %d", got.Balance, tt.want)
			}
		})
	}
}

func TestDebtAccountTransactionsAreInverted(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)
	debt := testutil.CreateTestDebtAccount(t, db, user.ID, 10000)

	// An expense on a debt account increases what is owed; editing and deleting reverse it.
	tx, err := txSvc.CreateTransaction(user.ID, debt.ID, nil, models.TransactionTypeExpense, 3000, "Charge", time.Now())
	testutil.AssertNoError(t, err)
	assertBalance(t, acctSvc, user.ID, debt.ID, 13000)

	amt := int64(5000)
	_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Amount: &amt})
	testutil.AssertNoError(t, err)
	assertBalance(t, acctSvc, user.ID, debt.ID, 15000)

	testutil.AssertNoError(t, txSvc.DeleteTransaction(user.ID, tx.ID))
	assertBalance(t, acctSvc, user.ID, debt.ID, 10000)
}

func TestCreateTransfer_DebtSourceExemptFromInsufficientBalance(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	txSvc := NewTransactionService(db, acctSvc, nil)
	user := testutil.CreateTestUser(t, db)
	debt := testutil.CreateTestDebtAccount(t, db, user.ID, 0)
	cash := testutil.CreateTestCashAccount(t, db, user.ID)

	// Borrowing from a debt account (balance 0) must not fail the balance check.
	_, err := txSvc.CreateTransfer(user.ID, debt.ID, cash.ID, 4000, "Draw", time.Now())
	testutil.AssertNoError(t, err)
	assertBalance(t, acctSvc, user.ID, debt.ID, 4000)
	assertBalance(t, acctSvc, user.ID, cash.ID, 4000)
}

func TestReverseType(t *testing.T) {
	tests := []struct {
		in      models.TransactionType
		want    models.TransactionType
		wantErr bool
	}{
		{models.TransactionTypeIncome, models.TransactionTypeExpense, false},
		{models.TransactionTypeExpense, models.TransactionTypeIncome, false},
		{models.TransactionTypeReimbursement, models.TransactionTypeExpense, false},
		{models.TransactionTypeTransfer, "", true},
		{models.TransactionTypeInvestment, "", true},
		{"bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			got, err := reverseType(tt.in)
			if tt.wantErr {
				testutil.AssertAppError(t, err, apperrors.ErrInvalidTransactionType.Code)
				return
			}
			testutil.AssertNoError(t, err)
			if got != tt.want {
				t.Errorf("reverseType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReimbursementGuards(t *testing.T) {
	newReimbursement := func(t *testing.T) (TransactionServicer, AccountServicer, string, string, string) {
		t.Helper()
		db := testutil.SetupTestDB(t)
		t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		reimb := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeReimbursement, 2000)
		return txSvc, acctSvc, user.ID, account.ID, reimb.ID
	}

	t.Run("update_reimbursement_rejected", func(t *testing.T) {
		txSvc, acctSvc, userID, accountID, id := newReimbursement(t)
		amt := int64(100)
		_, err := txSvc.UpdateTransaction(userID, id, TransactionUpdateFields{Amount: &amt})
		testutil.AssertAppError(t, err, apperrors.ErrTransactionNotEditable.Code)
		assertBalance(t, acctSvc, userID, accountID, 10000)
	})

	t.Run("delete_reimbursement_rejected", func(t *testing.T) {
		txSvc, acctSvc, userID, accountID, id := newReimbursement(t)
		err := txSvc.DeleteTransaction(userID, id)
		testutil.AssertAppError(t, err, apperrors.ErrTransactionNotEditable.Code)
		assertBalance(t, acctSvc, userID, accountID, 10000)
	})

	t.Run("change_type_to_reimbursement_rejected", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 1000, "x", time.Now())
		testutil.AssertNoError(t, err)

		newType := models.TransactionTypeReimbursement
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &newType})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidTypeChange.Code)
		assertBalance(t, acctSvc, user.ID, account.ID, 9000)
	})
}

func assertBalance(t *testing.T, svc AccountServicer, userID, accountID string, want int64) {
	t.Helper()
	got, err := svc.GetAccountByID(userID, accountID)
	testutil.AssertNoError(t, err)
	if got.Balance != want {
		t.Errorf("balance = %d, want %d", got.Balance, want)
	}
}
