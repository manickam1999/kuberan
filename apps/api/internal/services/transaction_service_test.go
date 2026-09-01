package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateTransaction(t *testing.T) {
	t.Run("income_increases_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "Salary", time.Now())
		testutil.AssertNoError(t, err)

		if tx.ID == "" {
			t.Fatal("expected non-empty transaction ID")
		}
		if tx.Amount != 5000 {
			t.Errorf("expected amount 5000, got %d", tx.Amount)
		}

		// Verify balance increased
		updated, err := acctSvc.GetAccountByID(user.ID, account.ID)
		testutil.AssertNoError(t, err)
		if updated.Balance != 5000 {
			t.Errorf("expected balance 5000, got %d", updated.Balance)
		}
	})

	t.Run("expense_decreases_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "Lunch", time.Now())
		testutil.AssertNoError(t, err)

		updated, err := acctSvc.GetAccountByID(user.ID, account.ID)
		testutil.AssertNoError(t, err)
		if updated.Balance != 7000 {
			t.Errorf("expected balance 7000, got %d", updated.Balance)
		}
	})

	t.Run("zero_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 0, "", time.Now())
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("negative_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, -100, "", time.Now())
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("zero_account_id", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)

		_, err := txSvc.CreateTransaction("1", "", nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("invalid_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		_, err := txSvc.CreateTransaction(user.ID, "99999", nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("wrong_user_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)

		_, err := txSvc.CreateTransaction(user2.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("with_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 500, "Coffee", time.Now())
		testutil.AssertNoError(t, err)

		if tx.CategoryID == nil || *tx.CategoryID != cat.ID {
			t.Error("expected category ID to be set")
		}
	})

	t.Run("default_date_when_zero", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Time{})
		testutil.AssertNoError(t, err)

		if tx.Date.IsZero() {
			t.Error("expected date to be defaulted to now, got zero")
		}
	})
}

func TestCreateTransfer(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransfer(user.ID, from.ID, to.ID, 3000, "Transfer", time.Now())
		testutil.AssertNoError(t, err)

		if tx.Type != models.TransactionTypeTransfer {
			t.Errorf("expected type transfer, got %s", tx.Type)
		}
		if tx.ToAccountID == nil || *tx.ToAccountID != to.ID {
			t.Error("expected ToAccountID to be set")
		}

		// Verify from-account balance decreased
		fromUpdated, err := acctSvc.GetAccountByID(user.ID, from.ID)
		testutil.AssertNoError(t, err)
		if fromUpdated.Balance != 7000 {
			t.Errorf("expected from-balance 7000, got %d", fromUpdated.Balance)
		}

		// Verify to-account balance increased
		toUpdated, err := acctSvc.GetAccountByID(user.ID, to.ID)
		testutil.AssertNoError(t, err)
		if toUpdated.Balance != 3000 {
			t.Errorf("expected to-balance 3000, got %d", toUpdated.Balance)
		}
	})

	t.Run("same_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		_, err := txSvc.CreateTransfer(user.ID, account.ID, account.ID, 1000, "", time.Now())
		testutil.AssertAppError(t, err, "SAME_ACCOUNT_TRANSFER")
	})

	t.Run("insufficient_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransfer(user.ID, from.ID, to.ID, 5000, "", time.Now())
		testutil.AssertAppError(t, err, "INSUFFICIENT_BALANCE")

		// Verify balances unchanged
		fromUpdated, err := acctSvc.GetAccountByID(user.ID, from.ID)
		testutil.AssertNoError(t, err)
		if fromUpdated.Balance != 1000 {
			t.Errorf("expected from-balance unchanged at 1000, got %d", fromUpdated.Balance)
		}
	})

	t.Run("zero_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransfer(user.ID, from.ID, to.ID, 0, "", time.Now())
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("invalid_from_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransfer(user.ID, "99999", to.ID, 1000, "", time.Now())
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("invalid_to_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		_, err := txSvc.CreateTransfer(user.ID, from.ID, "99999", 1000, "", time.Now())
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
}

func TestGetTransactionByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		created := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1000)

		tx, err := txSvc.GetTransactionByID(user.ID, created.ID)
		testutil.AssertNoError(t, err)

		if tx.ID != created.ID {
			t.Errorf("expected transaction ID %s, got %s", created.ID, tx.ID)
		}
		if tx.Amount != 1000 {
			t.Errorf("expected amount 1000, got %d", tx.Amount)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		_, err := txSvc.GetTransactionByID(user.ID, "99999")
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)
		created := testutil.CreateTestTransaction(t, db, user1.ID, account.ID, models.TransactionTypeIncome, 1000)

		_, err := txSvc.GetTransactionByID(user2.ID, created.ID)
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})
}

func TestGetAccountTransactions(t *testing.T) {
	t.Run("returns_account_transactions", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 500)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetAccountTransactions(user.ID, account.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 transactions, got %d", result.TotalItems)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		for i := 0; i < 5; i++ {
			testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, int64((i+1)*1000))
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := txSvc.GetAccountTransactions(user.ID, account.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected total 5, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page, got %d", len(result.Data))
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
	})

	t.Run("filter_by_type", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 2000)

		incomeType := models.TransactionTypeIncome
		filter := TransactionFilter{Type: &incomeType}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetAccountTransactions(user.ID, account.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 income transactions, got %d", result.TotalItems)
		}
	})

	t.Run("filter_by_amount_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 3000)

		minAmt := int64(1000)
		maxAmt := int64(2000)
		filter := TransactionFilter{MinAmount: &minAmt, MaxAmount: &maxAmt}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetAccountTransactions(user.ID, account.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 transaction in range, got %d", result.TotalItems)
		}
	})

	t.Run("filter_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		now := time.Now()
		old := &models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 1000,
			Date: now.AddDate(0, -2, 0),
		}
		recent := &models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 2000,
			Date: now.AddDate(0, 0, -1),
		}
		db.Create(old)
		db.Create(recent)

		fromDate := now.AddDate(0, -1, 0)
		filter := TransactionFilter{FromDate: &fromDate}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetAccountTransactions(user.ID, account.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 recent transaction, got %d", result.TotalItems)
		}
	})

	t.Run("invalid_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		_, err := txSvc.GetAccountTransactions(user.ID, "99999", page, TransactionFilter{})
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
}

func TestGetUserTransactions(t *testing.T) {
	t.Run("lists_all_transactions_across_accounts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		acct1 := testutil.CreateTestCashAccount(t, db, user.ID)
		acct2 := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, acct1.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user.ID, acct2.ID, models.TransactionTypeIncome, 2000)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 transactions across accounts, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_type", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 2000)

		expenseType := models.TransactionTypeExpense
		filter := TransactionFilter{Type: &expenseType}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 expense transaction, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		now := time.Now()
		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 1000,
			Date: now.AddDate(0, -3, 0),
		})
		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 2000,
			Date: now.AddDate(0, 0, -1),
		})

		fromDate := now.AddDate(0, -1, 0)
		toDate := now
		filter := TransactionFilter{FromDate: &fromDate, ToDate: &toDate}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 transaction in date range, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeExpense, Amount: 500,
			CategoryID: &cat.ID, Date: time.Now(),
		})
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 300)

		filter := TransactionFilter{CategoryID: &cat.ID}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 transaction with category, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_amount_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1500)
		testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 3000)

		minAmt := int64(1000)
		maxAmt := int64(2000)
		filter := TransactionFilter{MinAmount: &minAmt, MaxAmount: &maxAmt}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 transaction in amount range, got %d", result.TotalItems)
		}
	})

	t.Run("filters_by_account_id", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		acct1 := testutil.CreateTestCashAccount(t, db, user.ID)
		acct2 := testutil.CreateTestCashAccount(t, db, user.ID)

		testutil.CreateTestTransaction(t, db, user.ID, acct1.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user.ID, acct1.ID, models.TransactionTypeIncome, 2000)
		testutil.CreateTestTransaction(t, db, user.ID, acct2.ID, models.TransactionTypeIncome, 3000)

		filter := TransactionFilter{AccountID: &acct1.ID}
		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, filter)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 transactions for account 1, got %d", result.TotalItems)
		}
	})

	t.Run("paginates_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		for i := 0; i < 5; i++ {
			testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, int64((i+1)*1000))
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := txSvc.GetUserTransactions(user.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected total 5, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page, got %d", len(result.Data))
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
		if result.Page != 1 {
			t.Errorf("expected page 1, got %d", result.Page)
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		acct1 := testutil.CreateTestCashAccount(t, db, user1.ID)
		acct2 := testutil.CreateTestCashAccount(t, db, user2.ID)

		testutil.CreateTestTransaction(t, db, user1.ID, acct1.ID, models.TransactionTypeIncome, 1000)
		testutil.CreateTestTransaction(t, db, user2.ID, acct2.ID, models.TransactionTypeIncome, 2000)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user1.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 transaction for user1, got %d", result.TotalItems)
		}
	})

	t.Run("orders_by_date_desc", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		now := time.Now()
		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 1000,
			Description: "oldest", Date: now.AddDate(0, 0, -3),
		})
		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 3000,
			Description: "newest", Date: now,
		})
		db.Create(&models.Transaction{
			UserID: user.ID, AccountID: account.ID,
			Type: models.TransactionTypeIncome, Amount: 2000,
			Description: "middle", Date: now.AddDate(0, 0, -1),
		})

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		if len(result.Data) != 3 {
			t.Fatalf("expected 3 transactions, got %d", len(result.Data))
		}
		if result.Data[0].Description != "newest" {
			t.Errorf("expected first transaction to be 'newest', got %q", result.Data[0].Description)
		}
		if result.Data[2].Description != "oldest" {
			t.Errorf("expected last transaction to be 'oldest', got %q", result.Data[2].Description)
		}
	})

	t.Run("populates_attachments_count", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		withReceipts := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
		without := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 500)

		for i := 0; i < 2; i++ {
			db.Create(&models.TransactionAttachment{
				UserID: user.ID, TransactionID: withReceipts.ID,
				StorageKey: "key", ContentType: "image/jpeg", ByteSize: 10,
				ChecksumSHA256: "x",
			})
		}

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := txSvc.GetUserTransactions(user.ID, page, TransactionFilter{})
		testutil.AssertNoError(t, err)

		counts := map[string]int{}
		for _, tx := range result.Data {
			counts[tx.ID] = tx.AttachmentsCount
		}
		if counts[withReceipts.ID] != 2 {
			t.Errorf("expected attachments_count 2, got %d", counts[withReceipts.ID])
		}
		if counts[without.ID] != 0 {
			t.Errorf("expected attachments_count 0, got %d", counts[without.ID])
		}
	})
}

func TestDeleteTransaction(t *testing.T) {
	t.Run("income_reversal", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "Income", time.Now())
		testutil.AssertNoError(t, err)

		// Verify balance increased
		updated, _ := acctSvc.GetAccountByID(user.ID, account.ID)
		if updated.Balance != 5000 {
			t.Fatalf("expected balance 5000 after income, got %d", updated.Balance)
		}

		// Delete the income transaction
		err = txSvc.DeleteTransaction(user.ID, tx.ID)
		testutil.AssertNoError(t, err)

		// Balance should be back to 0
		updated, _ = acctSvc.GetAccountByID(user.ID, account.ID)
		if updated.Balance != 0 {
			t.Errorf("expected balance 0 after deleting income, got %d", updated.Balance)
		}
	})

	t.Run("expense_reversal", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "Expense", time.Now())
		testutil.AssertNoError(t, err)

		// Verify balance decreased
		updated, _ := acctSvc.GetAccountByID(user.ID, account.ID)
		if updated.Balance != 7000 {
			t.Fatalf("expected balance 7000 after expense, got %d", updated.Balance)
		}

		// Delete the expense
		err = txSvc.DeleteTransaction(user.ID, tx.ID)
		testutil.AssertNoError(t, err)

		// Balance should be restored
		updated, _ = acctSvc.GetAccountByID(user.ID, account.ID)
		if updated.Balance != 10000 {
			t.Errorf("expected balance 10000 after deleting expense, got %d", updated.Balance)
		}
	})

	t.Run("transfer_reversal", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransfer(user.ID, from.ID, to.ID, 4000, "Transfer", time.Now())
		testutil.AssertNoError(t, err)

		// Verify balances after transfer
		fromUpdated, _ := acctSvc.GetAccountByID(user.ID, from.ID)
		toUpdated, _ := acctSvc.GetAccountByID(user.ID, to.ID)
		if fromUpdated.Balance != 6000 {
			t.Fatalf("expected from-balance 6000, got %d", fromUpdated.Balance)
		}
		if toUpdated.Balance != 4000 {
			t.Fatalf("expected to-balance 4000, got %d", toUpdated.Balance)
		}

		// Delete the transfer
		err = txSvc.DeleteTransaction(user.ID, tx.ID)
		testutil.AssertNoError(t, err)

		// Balances should be restored
		fromUpdated, _ = acctSvc.GetAccountByID(user.ID, from.ID)
		toUpdated, _ = acctSvc.GetAccountByID(user.ID, to.ID)
		if fromUpdated.Balance != 10000 {
			t.Errorf("expected from-balance 10000 after delete, got %d", fromUpdated.Balance)
		}
		if toUpdated.Balance != 0 {
			t.Errorf("expected to-balance 0 after delete, got %d", toUpdated.Balance)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		err := txSvc.DeleteTransaction(user.ID, "99999")
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)

		tx, err := txSvc.CreateTransaction(user1.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertNoError(t, err)

		err = txSvc.DeleteTransaction(user2.ID, tx.ID)
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})
}

func TestUpdateTransaction(t *testing.T) {
	t.Run("updates_amount_adjusts_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "Salary", time.Now())
		testutil.AssertNoError(t, err)

		// Balance should be 5000
		acct, _ := acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 5000 {
			t.Fatalf("expected balance 5000, got %d", acct.Balance)
		}

		// Update amount from 5000 to 3000
		newAmount := int64(3000)
		updated, err := txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Amount: &newAmount})
		testutil.AssertNoError(t, err)

		if updated.Amount != 3000 {
			t.Errorf("expected updated amount 3000, got %d", updated.Amount)
		}

		// Balance should be 3000 (reversed 5000 income, then applied 3000 income)
		acct, _ = acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 3000 {
			t.Errorf("expected balance 3000, got %d", acct.Balance)
		}
	})

	t.Run("updates_type_income_to_expense", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "Income", time.Now())
		testutil.AssertNoError(t, err)

		// Verify balance is now 15000 (10000 initial + 5000 income)
		acct, _ := acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 15000 {
			t.Fatalf("expected balance 15000, got %d", acct.Balance)
		}

		// Change type to expense
		expenseType := models.TransactionTypeExpense
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &expenseType})
		testutil.AssertNoError(t, err)

		// Balance: reverse income(5000) → 15000 - 5000 = 10000, then apply expense(5000) → 10000 - 5000 = 5000
		acct, _ = acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 5000 {
			t.Errorf("expected balance 5000, got %d", acct.Balance)
		}
	})

	t.Run("updates_type_expense_to_income", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "Expense", time.Now())
		testutil.AssertNoError(t, err)

		// Verify balance is now 7000 (10000 initial - 3000 expense)
		acct, _ := acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 7000 {
			t.Fatalf("expected balance 7000, got %d", acct.Balance)
		}

		// Change type to income
		incomeType := models.TransactionTypeIncome
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &incomeType})
		testutil.AssertNoError(t, err)

		// Balance: reverse expense(3000) → 7000 + 3000 = 10000, then apply income(3000) → 10000 + 3000 = 13000
		acct, _ = acctSvc.GetAccountByID(user.ID, account.ID)
		if acct.Balance != 13000 {
			t.Errorf("expected balance 13000, got %d", acct.Balance)
		}
	})

	t.Run("updates_account_id", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		acctA := testutil.CreateTestCashAccount(t, db, user.ID)
		acctB := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, acctA.ID, nil, models.TransactionTypeIncome, 5000, "Income", time.Now())
		testutil.AssertNoError(t, err)

		// A: 5000, B: 0
		a, _ := acctSvc.GetAccountByID(user.ID, acctA.ID)
		b, _ := acctSvc.GetAccountByID(user.ID, acctB.ID)
		if a.Balance != 5000 || b.Balance != 0 {
			t.Fatalf("expected A=5000 B=0, got A=%d B=%d", a.Balance, b.Balance)
		}

		// Move transaction to account B
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{AccountID: &acctB.ID})
		testutil.AssertNoError(t, err)

		// A: 0 (reversed), B: 5000 (applied)
		a, _ = acctSvc.GetAccountByID(user.ID, acctA.ID)
		b, _ = acctSvc.GetAccountByID(user.ID, acctB.ID)
		if a.Balance != 0 {
			t.Errorf("expected A balance 0, got %d", a.Balance)
		}
		if b.Balance != 5000 {
			t.Errorf("expected B balance 5000, got %d", b.Balance)
		}
	})

	t.Run("updates_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		cat1 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		cat2 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &cat1.ID, models.TransactionTypeExpense, 1000, "Expense", time.Now())
		testutil.AssertNoError(t, err)

		// Update to cat2
		cat2IDPtr := &cat2.ID
		updated, err := txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{CategoryID: &cat2IDPtr})
		testutil.AssertNoError(t, err)

		if updated.CategoryID == nil || *updated.CategoryID != cat2.ID {
			t.Errorf("expected category_id %s, got %v", cat2.ID, updated.CategoryID)
		}
	})

	t.Run("clears_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "Expense", time.Now())
		testutil.AssertNoError(t, err)

		// Clear category: double pointer with nil inner
		var nilStr *string
		updated, err := txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{CategoryID: &nilStr})
		testutil.AssertNoError(t, err)

		if updated.CategoryID != nil {
			t.Errorf("expected category_id nil, got %v", updated.CategoryID)
		}
	})

	t.Run("updates_description_and_date", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "Old desc", time.Now())
		testutil.AssertNoError(t, err)

		newDesc := "New description"
		newDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		updated, err := txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{
			Description: &newDesc,
			Date:        &newDate,
		})
		testutil.AssertNoError(t, err)

		if updated.Description != "New description" {
			t.Errorf("expected description 'New description', got %q", updated.Description)
		}
		if !updated.Date.Equal(newDate) {
			t.Errorf("expected date %v, got %v", newDate, updated.Date)
		}
	})

	t.Run("rejects_transfer_transaction", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		from := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10000)
		to := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransfer(user.ID, from.ID, to.ID, 3000, "Transfer", time.Now())
		testutil.AssertNoError(t, err)

		newAmount := int64(5000)
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Amount: &newAmount})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_EDITABLE")
	})

	t.Run("rejects_investment_transaction", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		// Create an investment-type transaction directly in DB
		investTx := &models.Transaction{
			UserID:    user.ID,
			AccountID: account.ID,
			Type:      models.TransactionTypeInvestment,
			Amount:    10000,
			Date:      time.Now(),
		}
		db.Create(investTx)

		newAmount := int64(5000)
		_, err := txSvc.UpdateTransaction(user.ID, investTx.ID, TransactionUpdateFields{Amount: &newAmount})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_EDITABLE")
	})

	t.Run("rejects_type_change_to_transfer", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertNoError(t, err)

		transferType := models.TransactionTypeTransfer
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &transferType})
		testutil.AssertAppError(t, err, "INVALID_TYPE_CHANGE")
	})

	t.Run("rejects_type_change_to_investment", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		tx, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertNoError(t, err)

		investType := models.TransactionTypeInvestment
		_, err = txSvc.UpdateTransaction(user.ID, tx.ID, TransactionUpdateFields{Type: &investType})
		testutil.AssertAppError(t, err, "INVALID_TYPE_CHANGE")
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)

		tx, err := txSvc.CreateTransaction(user1.ID, account.ID, nil, models.TransactionTypeIncome, 1000, "", time.Now())
		testutil.AssertNoError(t, err)

		newAmount := int64(2000)
		_, err = txSvc.UpdateTransaction(user2.ID, tx.ID, TransactionUpdateFields{Amount: &newAmount})
		testutil.AssertAppError(t, err, "TRANSACTION_NOT_FOUND")
	})
}

func TestGetSpendingByCategory(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	t.Run("groups_by_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		catA := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catB := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		// Two expenses for catA
		_, err := txSvc.CreateTransaction(user.ID, account.ID, &catA.ID, models.TransactionTypeExpense, 3000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &catA.ID, models.TransactionTypeExpense, 2000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)

		// One expense for catB
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &catB.ID, models.TransactionTypeExpense, 1500, "", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result.Items))
		}
		if result.TotalSpent != 6500 {
			t.Errorf("expected total_spent 6500, got %d", result.TotalSpent)
		}
		// First item should be catA (5000 > 1500)
		if result.Items[0].Total != 5000 {
			t.Errorf("expected first item total 5000, got %d", result.Items[0].Total)
		}
		if result.Items[1].Total != 1500 {
			t.Errorf("expected second item total 1500, got %d", result.Items[1].Total)
		}
	})

	t.Run("handles_uncategorized", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2500, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Items))
		}
		if result.Items[0].CategoryName != "Uncategorized" {
			t.Errorf("expected category_name 'Uncategorized', got %q", result.Items[0].CategoryName)
		}
		if result.Items[0].CategoryColor != "#9CA3AF" {
			t.Errorf("expected color '#9CA3AF', got %q", result.Items[0].CategoryColor)
		}
		if result.Items[0].CategoryID != nil {
			t.Errorf("expected nil category_id, got %v", result.Items[0].CategoryID)
		}
	})

	t.Run("filters_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		// January expense (out of range for February query)
		jan := time.Date(now.Year(), 1, 15, 12, 0, 0, 0, time.UTC)
		_, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "", jan)
		testutil.AssertNoError(t, err)

		// February expense (in range)
		feb := time.Date(now.Year(), 2, 15, 12, 0, 0, 0, time.UTC)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 2000, "", feb)
		testutil.AssertNoError(t, err)

		febFrom := time.Date(now.Year(), 2, 1, 0, 0, 0, 0, time.UTC)
		febTo := time.Date(now.Year(), 2, 28, 23, 59, 59, 0, time.UTC)
		result, err := txSvc.GetSpendingByCategory(user.ID, febFrom, febTo)
		testutil.AssertNoError(t, err)

		if result.TotalSpent != 2000 {
			t.Errorf("expected total_spent 2000, got %d", result.TotalSpent)
		}
	})

	t.Run("excludes_non_expense_types", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		account2 := testutil.CreateTestCashAccount(t, db, user.ID)

		// Income transaction
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		// Transfer transaction
		_, err = txSvc.CreateTransfer(user.ID, account.ID, account2.ID, 1000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalSpent != 0 {
			t.Errorf("expected total_spent 0, got %d", result.TotalSpent)
		}
		if len(result.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(result.Items))
		}
	})

	t.Run("returns_empty_for_no_expenses", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalSpent != 0 {
			t.Errorf("expected total_spent 0, got %d", result.TotalSpent)
		}
		if len(result.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(result.Items))
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeExpense, 3000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeExpense, 5000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(userA.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalSpent != 3000 {
			t.Errorf("expected total_spent 3000, got %d", result.TotalSpent)
		}
	})

	t.Run("generates_fallback_color_for_colorless_categories", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// CreateTestCategory creates categories without a color set
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Items))
		}
		// Color should not be empty — should be a palette fallback
		if result.Items[0].CategoryColor == "" {
			t.Error("expected non-empty fallback color for colorless category")
		}
		// Should be a valid hex color from the palette
		expectedColor := getCategoryColorFromID(cat.ID)
		if result.Items[0].CategoryColor != expectedColor {
			t.Errorf("expected fallback color %q, got %q", expectedColor, result.Items[0].CategoryColor)
		}
	})

	t.Run("uses_category_color_when_set", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// Create a category with an explicit color
		cat := &models.Category{
			UserID: user.ID,
			Name:   "Colored Category",
			Type:   models.CategoryTypeExpense,
			Color:  "#FF00FF",
		}
		if err := db.Create(cat).Error; err != nil {
			t.Fatalf("failed to create category: %v", err)
		}

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Items))
		}
		if result.Items[0].CategoryColor != "#FF00FF" {
			t.Errorf("expected color '#FF00FF', got %q", result.Items[0].CategoryColor)
		}
	})

	t.Run("sorts_by_total_descending", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		catSmall := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catMedium := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catLarge := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &catSmall.ID, models.TransactionTypeExpense, 1000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &catMedium.ID, models.TransactionTypeExpense, 3000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &catLarge.ID, models.TransactionTypeExpense, 5000, "", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetSpendingByCategory(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.Items))
		}
		if result.Items[0].Total != 5000 {
			t.Errorf("expected first item total 5000, got %d", result.Items[0].Total)
		}
		if result.Items[1].Total != 3000 {
			t.Errorf("expected second item total 3000, got %d", result.Items[1].Total)
		}
		if result.Items[2].Total != 1000 {
			t.Errorf("expected third item total 1000, got %d", result.Items[2].Total)
		}
	})
}

func TestGetCashflow(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	t.Run("splits_income_and_expenses_by_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		salary := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)
		dividends := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)
		rent := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &salary.ID, models.TransactionTypeIncome, 8000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &dividends.ID, models.TransactionTypeIncome, 2000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &rent.ID, models.TransactionTypeExpense, 3000, "", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetCashflow(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalIncome != 10000 {
			t.Errorf("expected total_income 10000, got %d", result.TotalIncome)
		}
		if result.TotalExpenses != 3000 {
			t.Errorf("expected total_expenses 3000, got %d", result.TotalExpenses)
		}
		if len(result.Income) != 2 {
			t.Fatalf("expected 2 income items, got %d", len(result.Income))
		}
		// Income sorted descending: salary (8000) before dividends (2000).
		if result.Income[0].Total != 8000 {
			t.Errorf("expected first income item 8000, got %d", result.Income[0].Total)
		}
		if len(result.Expenses) != 1 {
			t.Fatalf("expected 1 expense item, got %d", len(result.Expenses))
		}
		if result.Expenses[0].Total != 3000 {
			t.Errorf("expected expense item 3000, got %d", result.Expenses[0].Total)
		}
	})

	t.Run("excludes_transfers_and_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		account2 := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransfer(user.ID, account.ID, account2.ID, 1000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetCashflow(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalIncome != 5000 {
			t.Errorf("expected total_income 5000, got %d", result.TotalIncome)
		}
		if result.TotalExpenses != 0 {
			t.Errorf("expected total_expenses 0 (transfer excluded), got %d", result.TotalExpenses)
		}
	})

	t.Run("returns_empty_slices_when_no_activity", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		result, err := txSvc.GetCashflow(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.Income == nil || result.Expenses == nil {
			t.Fatal("expected non-nil empty slices for income and expenses")
		}
		if len(result.Income) != 0 || len(result.Expenses) != 0 {
			t.Errorf("expected empty slices, got income=%d expenses=%d", len(result.Income), len(result.Expenses))
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeIncome, 3000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeIncome, 9000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetCashflow(userA.ID, from, to)
		testutil.AssertNoError(t, err)

		if result.TotalIncome != 3000 {
			t.Errorf("expected total_income 3000, got %d", result.TotalIncome)
		}
	})
}

func TestGetMonthlySummary(t *testing.T) {
	now := time.Now()

	t.Run("returns_monthly_totals", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// Current month: income 10000, expense 5000
		curMonth := time.Date(now.Year(), now.Month(), 10, 12, 0, 0, 0, time.UTC)
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 10000, "", curMonth)
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 5000, "", curMonth)
		testutil.AssertNoError(t, err)

		// Previous month: income 8000, expense 3000
		prevMonth := curMonth.AddDate(0, -1, 0)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 8000, "", prevMonth)
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "", prevMonth)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetMonthlySummary(user.ID, 2)
		testutil.AssertNoError(t, err)

		if len(result) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result))
		}
		// First item is previous month (chronological order)
		if result[0].Income != 8000 {
			t.Errorf("expected prev month income 8000, got %d", result[0].Income)
		}
		if result[0].Expenses != 3000 {
			t.Errorf("expected prev month expenses 3000, got %d", result[0].Expenses)
		}
		// Second item is current month
		if result[1].Income != 10000 {
			t.Errorf("expected cur month income 10000, got %d", result[1].Income)
		}
		if result[1].Expenses != 5000 {
			t.Errorf("expected cur month expenses 5000, got %d", result[1].Expenses)
		}
	})

	t.Run("returns_zero_for_empty_months", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		result, err := txSvc.GetMonthlySummary(user.ID, 3)
		testutil.AssertNoError(t, err)

		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}
		for i, item := range result {
			if item.Income != 0 {
				t.Errorf("item[%d]: expected income 0, got %d", i, item.Income)
			}
			if item.Expenses != 0 {
				t.Errorf("item[%d]: expected expenses 0, got %d", i, item.Expenses)
			}
		}
	})

	t.Run("excludes_transfers_and_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		account2 := testutil.CreateTestCashAccount(t, db, user.ID)

		curMonth := time.Date(now.Year(), now.Month(), 10, 12, 0, 0, 0, time.UTC)

		// Transfer
		_, err := txSvc.CreateTransfer(user.ID, account.ID, account2.ID, 2000, "", curMonth)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetMonthlySummary(user.ID, 1)
		testutil.AssertNoError(t, err)

		if len(result) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result))
		}
		if result[0].Income != 0 {
			t.Errorf("expected income 0, got %d", result[0].Income)
		}
		if result[0].Expenses != 0 {
			t.Errorf("expected expenses 0, got %d", result[0].Expenses)
		}
	})

	t.Run("excludes_initial_balance_from_income", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		// CreateCashAccount with initial balance creates an income transaction with description "Initial balance"
		account, err := acctSvc.CreateCashAccount(user.ID, "Savings", "", "USD", 50000)
		testutil.AssertNoError(t, err)

		// Add a regular income transaction in the current month
		curMonth := time.Date(now.Year(), now.Month(), 10, 12, 0, 0, 0, time.UTC)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 7000, "Salary", curMonth)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetMonthlySummary(user.ID, 1)
		testutil.AssertNoError(t, err)

		if len(result) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result))
		}
		// Income should only include the 7000 salary, not the 50000 initial balance
		if result[0].Income != 7000 {
			t.Errorf("expected income 7000 (excluding initial balance), got %d", result[0].Income)
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		curMonth := time.Date(now.Year(), now.Month(), 10, 12, 0, 0, 0, time.UTC)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeIncome, 5000, "", curMonth)
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeIncome, 9000, "", curMonth)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetMonthlySummary(userA.ID, 1)
		testutil.AssertNoError(t, err)

		if len(result) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result))
		}
		if result[0].Income != 5000 {
			t.Errorf("expected income 5000, got %d", result[0].Income)
		}
	})
}

func TestGetDailySpending(t *testing.T) {
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 3, 23, 59, 59, 0, time.UTC)

	t.Run("returns_daily_totals", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// Day 1: two expenses (3000 + 2000 = 5000)
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "", time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2000, "", time.Date(2026, 2, 1, 14, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		// Day 3: one expense (1500)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 1500, "", time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySpending(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}
		if result[0].Date != "2026-02-01" || result[0].Total != 5000 {
			t.Errorf("day 1: expected date=2026-02-01 total=5000, got date=%s total=%d", result[0].Date, result[0].Total)
		}
		if result[1].Date != "2026-02-02" || result[1].Total != 0 {
			t.Errorf("day 2: expected date=2026-02-02 total=0, got date=%s total=%d", result[1].Date, result[1].Total)
		}
		if result[2].Date != "2026-02-03" || result[2].Total != 1500 {
			t.Errorf("day 3: expected date=2026-02-03 total=1500, got date=%s total=%d", result[2].Date, result[2].Total)
		}
	})

	t.Run("includes_zero_days", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		fiveDay := time.Date(2026, 2, 5, 23, 59, 59, 0, time.UTC)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 1000, "", time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySpending(user.ID, from, fiveDay)
		testutil.AssertNoError(t, err)

		if len(result) != 5 {
			t.Fatalf("expected 5 items, got %d", len(result))
		}
		for i := 1; i < 5; i++ {
			if result[i].Total != 0 {
				t.Errorf("day %d: expected total 0, got %d", i+1, result[i].Total)
			}
		}
	})

	t.Run("excludes_non_expense_types", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		account2 := testutil.CreateTestCashAccount(t, db, user.ID)

		day1 := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

		// Income
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 5000, "", day1)
		testutil.AssertNoError(t, err)

		// Transfer
		_, err = txSvc.CreateTransfer(user.ID, account.ID, account2.ID, 1000, "", day1)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySpending(user.ID, from, to)
		testutil.AssertNoError(t, err)

		for _, item := range result {
			if item.Total != 0 {
				t.Errorf("date %s: expected total 0, got %d", item.Date, item.Total)
			}
		}
	})

	t.Run("filters_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// Expense before range
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 1000, "", time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		// Expense after range
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2000, "", time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySpending(user.ID, from, to)
		testutil.AssertNoError(t, err)

		for _, item := range result {
			if item.Total != 0 {
				t.Errorf("date %s: expected total 0 (out-of-range expenses only), got %d", item.Date, item.Total)
			}
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		day1 := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeExpense, 3000, "", day1)
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeExpense, 7000, "", day1)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySpending(userA.ID, from, to)
		testutil.AssertNoError(t, err)

		if result[0].Total != 3000 {
			t.Errorf("expected day 1 total 3000 for userA, got %d", result[0].Total)
		}
	})
}

func TestGetDailySummary(t *testing.T) {
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 3, 23, 59, 59, 0, time.UTC)

	t.Run("returns_daily_income_and_expenses", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		// Day 1: income 10000, expense 3000
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 10000, "salary", time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 3000, "", time.Date(2026, 2, 1, 14, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		// Day 3: expense only (1500)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 1500, "", time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySummary(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if len(result) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result))
		}
		if result[0].Date != "2026-02-01" || result[0].Income != 10000 || result[0].Expenses != 3000 {
			t.Errorf("day 1: expected income=10000 expenses=3000, got income=%d expenses=%d", result[0].Income, result[0].Expenses)
		}
		if result[1].Date != "2026-02-02" || result[1].Income != 0 || result[1].Expenses != 0 {
			t.Errorf("day 2: expected income=0 expenses=0, got income=%d expenses=%d", result[1].Income, result[1].Expenses)
		}
		if result[2].Date != "2026-02-03" || result[2].Income != 0 || result[2].Expenses != 1500 {
			t.Errorf("day 3: expected income=0 expenses=1500, got income=%d expenses=%d", result[2].Income, result[2].Expenses)
		}
	})

	t.Run("excludes_initial_balance_from_income", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 100000, "Initial balance", time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySummary(user.ID, from, to)
		testutil.AssertNoError(t, err)

		if result[0].Income != 0 {
			t.Errorf("expected initial balance excluded from income, got %d", result[0].Income)
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		day1 := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeExpense, 3000, "", day1)
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeExpense, 7000, "", day1)
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetDailySummary(userA.ID, from, to)
		testutil.AssertNoError(t, err)

		if result[0].Expenses != 3000 {
			t.Errorf("expected day 1 expenses 3000 for userA, got %d", result[0].Expenses)
		}
	})
}

func TestGetTopExpenses(t *testing.T) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	t.Run("orders_by_amount_descending", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1500, "small", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 5000, "big", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 3000, "medium", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.Items))
		}
		if result.Items[0].Amount != 5000 || result.Items[1].Amount != 3000 || result.Items[2].Amount != 1500 {
			t.Errorf("expected amounts ordered [5000,3000,1500], got [%d,%d,%d]",
				result.Items[0].Amount, result.Items[1].Amount, result.Items[2].Amount)
		}
		if result.Items[0].AccountName != account.Name {
			t.Errorf("expected account_name %q, got %q", account.Name, result.Items[0].AccountName)
		}
	})

	t.Run("enforces_limit", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		for i := 1; i <= 5; i++ {
			_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, int64(i*1000), "", from.Add(time.Duration(i)*time.Hour))
			testutil.AssertNoError(t, err)
		}

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 2, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(result.Items))
		}
		if result.Items[0].Amount != 5000 || result.Items[1].Amount != 4000 {
			t.Errorf("expected top 2 amounts [5000,4000], got [%d,%d]", result.Items[0].Amount, result.Items[1].Amount)
		}
	})

	t.Run("excludes_non_expense_types", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		account2 := testutil.CreateTestCashAccount(t, db, user.ID)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeIncome, 9000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransfer(user.ID, account.ID, account2.ID, 8000, "", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 500, "only expense", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item (income/transfer excluded), got %d", len(result.Items))
		}
		if result.Items[0].Amount != 500 {
			t.Errorf("expected amount 500, got %d", result.Items[0].Amount)
		}
	})

	t.Run("filters_by_date_range", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		jan := time.Date(now.Year(), 1, 15, 12, 0, 0, 0, time.UTC)
		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 9999, "out of range", jan)
		testutil.AssertNoError(t, err)

		feb := time.Date(now.Year(), 2, 15, 12, 0, 0, 0, time.UTC)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 4000, "in range", feb)
		testutil.AssertNoError(t, err)

		febFrom := time.Date(now.Year(), 2, 1, 0, 0, 0, 0, time.UTC)
		febTo := time.Date(now.Year(), 2, 28, 23, 59, 59, 0, time.UTC)
		result, err := txSvc.GetTopExpenses(user.ID, febFrom, febTo, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item in range, got %d", len(result.Items))
		}
		if result.Items[0].Amount != 4000 {
			t.Errorf("expected amount 4000, got %d", result.Items[0].Amount)
		}
	})

	t.Run("handles_uncategorized", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 2500, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Items))
		}
		if result.Items[0].CategoryName != "Uncategorized" {
			t.Errorf("expected category_name 'Uncategorized', got %q", result.Items[0].CategoryName)
		}
		if result.Items[0].CategoryColor != "#9CA3AF" {
			t.Errorf("expected color '#9CA3AF', got %q", result.Items[0].CategoryColor)
		}
		if result.Items[0].CategoryID != nil {
			t.Errorf("expected nil category_id, got %v", result.Items[0].CategoryID)
		}
	})

	t.Run("generates_fallback_color_for_colorless_categories", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 1000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.Items))
		}
		expectedColor := getCategoryColorFromID(cat.ID)
		if result.Items[0].CategoryColor != expectedColor {
			t.Errorf("expected fallback color %q, got %q", expectedColor, result.Items[0].CategoryColor)
		}
	})

	t.Run("user_isolation", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		userA := testutil.CreateTestUser(t, db)
		userB := testutil.CreateTestUser(t, db)
		accountA := testutil.CreateTestCashAccountWithBalance(t, db, userA.ID, 100000)
		accountB := testutil.CreateTestCashAccountWithBalance(t, db, userB.ID, 100000)

		_, err := txSvc.CreateTransaction(userA.ID, accountA.ID, nil, models.TransactionTypeExpense, 3000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(userB.ID, accountB.ID, nil, models.TransactionTypeExpense, 9000, "", from.Add(time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(userA.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item for userA, got %d", len(result.Items))
		}
		if result.Items[0].Amount != 3000 {
			t.Errorf("expected amount 3000 for userA, got %d", result.Items[0].Amount)
		}
	})

	t.Run("returns_empty_for_no_expenses", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, nil)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 0 {
			t.Errorf("expected 0 items, got %d", len(result.Items))
		}
	})

	t.Run("filters_by_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		acctSvc := NewAccountService(db)
		txSvc := NewTransactionService(db, acctSvc, nil)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		catA := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catB := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := txSvc.CreateTransaction(user.ID, account.ID, &catA.ID, models.TransactionTypeExpense, 9000, "catA big", from.Add(time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, &catB.ID, models.TransactionTypeExpense, 5000, "catB", from.Add(2*time.Hour))
		testutil.AssertNoError(t, err)
		_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 20000, "uncategorized biggest", from.Add(3*time.Hour))
		testutil.AssertNoError(t, err)

		result, err := txSvc.GetTopExpenses(user.ID, from, to, 10, &catA.ID)
		testutil.AssertNoError(t, err)

		if len(result.Items) != 1 {
			t.Fatalf("expected 1 item scoped to catA, got %d", len(result.Items))
		}
		if result.Items[0].Amount != 9000 || result.Items[0].CategoryID == nil || *result.Items[0].CategoryID != catA.ID {
			t.Errorf("expected catA's 9000 expense, got amount=%d categoryID=%v", result.Items[0].Amount, result.Items[0].CategoryID)
		}
	})
}
