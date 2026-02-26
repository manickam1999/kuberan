package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateCashAccount(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateCashAccount(user.ID, "Savings", "My savings", "USD", 0)
		testutil.AssertNoError(t, err)

		if account.ID == "" {
			t.Fatal("expected non-empty account ID")
		}
		if account.Name != "Savings" {
			t.Errorf("expected name Savings, got %s", account.Name)
		}
		if account.Type != models.AccountTypeCash {
			t.Errorf("expected type cash, got %s", account.Type)
		}
		if account.Currency != "USD" {
			t.Errorf("expected currency USD, got %s", account.Currency)
		}
		if !account.IsActive {
			t.Error("expected account to be active")
		}
	})

	t.Run("with_initial_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateCashAccount(user.ID, "Checking", "", "USD", 5000)
		testutil.AssertNoError(t, err)

		if account.Balance != 5000 {
			t.Errorf("expected balance 5000, got %d", account.Balance)
		}

		// Verify initial transaction was created atomically
		var txCount int64
		db.Model(&models.Transaction{}).Where("account_id = ?", account.ID).Count(&txCount)
		if txCount != 1 {
			t.Errorf("expected 1 initial transaction, got %d", txCount)
		}

		var tx models.Transaction
		db.Where("account_id = ?", account.ID).First(&tx)
		if tx.Type != models.TransactionTypeIncome {
			t.Errorf("expected initial transaction type income, got %s", tx.Type)
		}
		if tx.Amount != 5000 {
			t.Errorf("expected initial transaction amount 5000, got %d", tx.Amount)
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCashAccount(user.ID, "", "", "USD", 0)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("default_currency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateCashAccount(user.ID, "No Currency", "", "", 0)
		testutil.AssertNoError(t, err)

		if account.Currency != "USD" {
			t.Errorf("expected default currency USD, got %s", account.Currency)
		}
	})
}

func TestGetUserAccounts(t *testing.T) {
	t.Run("returns_user_accounts_only", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)

		testutil.CreateTestCashAccount(t, db, user1.ID)
		testutil.CreateTestCashAccount(t, db, user1.ID)
		testutil.CreateTestCashAccount(t, db, user2.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user1.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 accounts for user1, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 accounts in data, got %d", len(result.Data))
		}
	})

	t.Run("excludes_inactive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		active := testutil.CreateTestCashAccount(t, db, user.ID)
		inactive := testutil.CreateTestCashAccount(t, db, user.ID)
		db.Model(inactive).Update("is_active", false)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 active account, got %d", result.TotalItems)
		}
		if result.Data[0].ID != active.ID {
			t.Errorf("expected active account ID %d, got %d", active.ID, result.Data[0].ID)
		}
	})
}

func TestGetAccountByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		created := testutil.CreateTestCashAccount(t, db, user.ID)

		account, err := svc.GetAccountByID(user.ID, created.ID)
		testutil.AssertNoError(t, err)

		if account.ID != created.ID {
			t.Errorf("expected account ID %d, got %d", created.ID, account.ID)
		}
		if account.Name != created.Name {
			t.Errorf("expected name %s, got %s", created.Name, account.Name)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetAccountByID(user.ID, "99999")
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)

		_, err := svc.GetAccountByID(user2.ID, account.ID)
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
}

func TestUpdateAccount(t *testing.T) {
	t.Run("updates_cash_account_name_and_description", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		name := "New Name"
		desc := "New Description"
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			Name:        &name,
			Description: &desc,
		})
		testutil.AssertNoError(t, err)

		if updated.Name != "New Name" {
			t.Errorf("expected name 'New Name', got %s", updated.Name)
		}
		if updated.Description != "New Description" {
			t.Errorf("expected description 'New Description', got %s", updated.Description)
		}
	})

	t.Run("updates_investment_account_broker", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		broker := "Schwab"
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			Broker: &broker,
		})
		testutil.AssertNoError(t, err)

		if updated.Broker != "Schwab" {
			t.Errorf("expected broker 'Schwab', got %s", updated.Broker)
		}
	})

	t.Run("updates_investment_account_number", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		acctNum := "XYZ-789"
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			AccountNumber: &acctNum,
		})
		testutil.AssertNoError(t, err)

		if updated.AccountNumber != "XYZ-789" {
			t.Errorf("expected account_number 'XYZ-789', got %s", updated.AccountNumber)
		}
	})

	t.Run("updates_credit_card_interest_rate", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

		rate := 22.5
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			InterestRate: &rate,
		})
		testutil.AssertNoError(t, err)

		if updated.InterestRate != 22.5 {
			t.Errorf("expected interest_rate 22.5, got %f", updated.InterestRate)
		}
	})

	t.Run("updates_credit_card_credit_limit", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

		limit := int64(1000000)
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			CreditLimit: &limit,
		})
		testutil.AssertNoError(t, err)

		if updated.CreditLimit != 1000000 {
			t.Errorf("expected credit_limit 1000000, got %d", updated.CreditLimit)
		}
	})

	t.Run("updates_credit_card_due_date", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

		dueDate := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			DueDate: &dueDate,
		})
		testutil.AssertNoError(t, err)

		if updated.DueDate.Year() != 2026 || updated.DueDate.Month() != 4 || updated.DueDate.Day() != 15 {
			t.Errorf("expected due_date 2026-04-15, got %v", updated.DueDate)
		}
	})

	t.Run("ignores_broker_for_cash_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		broker := "Fidelity"
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			Broker: &broker,
		})
		testutil.AssertNoError(t, err)

		if updated.Broker != "" {
			t.Errorf("expected broker to be empty for cash account, got %s", updated.Broker)
		}
	})

	t.Run("ignores_credit_limit_for_investment_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)

		limit := int64(100000)
		updated, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			CreditLimit: &limit,
		})
		testutil.AssertNoError(t, err)

		if updated.CreditLimit != 0 {
			t.Errorf("expected credit_limit 0 for investment account, got %d", updated.CreditLimit)
		}
	})

	t.Run("toggles_is_active", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)

		inactive := false
		_, err := svc.UpdateAccount(user.ID, account.ID, AccountUpdateFields{
			IsActive: &inactive,
		})
		testutil.AssertNoError(t, err)

		// Verify in DB (GetAccountByID filters active=true, so query directly)
		var dbAccount models.Account
		db.First(&dbAccount, account.ID)
		if dbAccount.IsActive {
			t.Error("expected account to be inactive")
		}
	})

	t.Run("returns_error_for_nonexistent_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		name := "Test"
		_, err := svc.UpdateAccount(user.ID, "99999", AccountUpdateFields{
			Name: &name,
		})
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})

	t.Run("returns_error_for_wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user1.ID)

		name := "Hacked"
		_, err := svc.UpdateAccount(user2.ID, account.ID, AccountUpdateFields{
			Name: &name,
		})
		testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
	})
}

func TestUpdateAccountBalance(t *testing.T) {
	t.Run("income_adds", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		err := svc.UpdateAccountBalance(db, account, models.TransactionTypeIncome, 500)
		testutil.AssertNoError(t, err)

		if account.Balance != 1500 {
			t.Errorf("expected balance 1500 after income, got %d", account.Balance)
		}

		// Verify persisted to DB
		var dbAccount models.Account
		db.First(&dbAccount, account.ID)
		if dbAccount.Balance != 1500 {
			t.Errorf("expected DB balance 1500, got %d", dbAccount.Balance)
		}
	})

	t.Run("expense_subtracts", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		err := svc.UpdateAccountBalance(db, account, models.TransactionTypeExpense, 300)
		testutil.AssertNoError(t, err)

		if account.Balance != 700 {
			t.Errorf("expected balance 700 after expense, got %d", account.Balance)
		}

		// Verify persisted to DB
		var dbAccount models.Account
		db.First(&dbAccount, account.ID)
		if dbAccount.Balance != 700 {
			t.Errorf("expected DB balance 700, got %d", dbAccount.Balance)
		}
	})
}

func TestCreateInvestmentAccount(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateInvestmentAccount(user.ID, "Brokerage", "My investments", "USD", "Fidelity", "123456")
		testutil.AssertNoError(t, err)

		if account.Type != models.AccountTypeInvestment {
			t.Errorf("expected type investment, got %s", account.Type)
		}
		if account.Broker != "Fidelity" {
			t.Errorf("expected broker Fidelity, got %s", account.Broker)
		}
		if account.AccountNumber != "123456" {
			t.Errorf("expected account number 123456, got %s", account.AccountNumber)
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateInvestmentAccount(user.ID, "", "", "USD", "", "")
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("default_currency", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateInvestmentAccount(user.ID, "Invest", "", "", "", "")
		testutil.AssertNoError(t, err)

		if account.Currency != "USD" {
			t.Errorf("expected default currency USD, got %s", account.Currency)
		}
	})
}

func TestCreateCreditCardAccount(t *testing.T) {
	t.Run("creates_credit_card_account", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		dueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		account, err := svc.CreateCreditCardAccount(user.ID, "Visa", "My credit card", "USD", 500000, 19.99, &dueDate)
		testutil.AssertNoError(t, err)

		if account.ID == "" {
			t.Fatal("expected non-empty account ID")
		}
		if account.Type != models.AccountTypeCreditCard {
			t.Errorf("expected type credit_card, got %s", account.Type)
		}
		if account.Balance != 0 {
			t.Errorf("expected balance 0, got %d", account.Balance)
		}
		if account.CreditLimit != 500000 {
			t.Errorf("expected credit limit 500000, got %d", account.CreditLimit)
		}
		if account.InterestRate != 19.99 {
			t.Errorf("expected interest rate 19.99, got %f", account.InterestRate)
		}
		if !account.IsActive {
			t.Error("expected account to be active")
		}
	})

	t.Run("defaults_currency_to_usd", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		account, err := svc.CreateCreditCardAccount(user.ID, "Amex", "", "", 0, 0, nil)
		testutil.AssertNoError(t, err)

		if account.Currency != "USD" {
			t.Errorf("expected default currency USD, got %s", account.Currency)
		}
	})

	t.Run("returns_error_for_empty_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCreditCardAccount(user.ID, "", "", "USD", 0, 0, nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})
}

func TestGetUserAccountsInvestmentBalance(t *testing.T) {
	t.Run("enriches_investment_account_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		// Create investment: 10 shares, cost basis $1000
		testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		// Create security price: $150.00 per share
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 15000, time.Now())

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user.ID, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 account, got %d", len(result.Data))
		}
		// Expected balance = 10 shares * $150.00 = $1500.00 = 150000 cents
		if result.Data[0].Balance != 150000 {
			t.Errorf("expected balance 150000, got %d", result.Data[0].Balance)
		}
	})

	t.Run("leaves_cash_account_balance_unchanged", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 5000)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user.ID, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 account, got %d", len(result.Data))
		}
		if result.Data[0].Balance != 5000 {
			t.Errorf("expected cash balance 5000, got %d", result.Data[0].Balance)
		}
	})

	t.Run("handles_investment_account_with_no_investments", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		testutil.CreateTestInvestmentAccount(t, db, user.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user.ID, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 account, got %d", len(result.Data))
		}
		if result.Data[0].Balance != 0 {
			t.Errorf("expected balance 0 for account with no investments, got %d", result.Data[0].Balance)
		}
	})

	t.Run("handles_investment_with_no_security_price", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		// Create investment but no security price
		testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserAccounts(user.ID, page)
		testutil.AssertNoError(t, err)

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 account, got %d", len(result.Data))
		}
		if result.Data[0].Balance != 0 {
			t.Errorf("expected balance 0 for investment with no price, got %d", result.Data[0].Balance)
		}
	})
}

func TestGetAccountByIDInvestmentBalance(t *testing.T) {
	t.Run("enriches_investment_account_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestInvestmentAccount(t, db, user.ID)
		sec := testutil.CreateTestSecurity(t, db)

		// Create investment: 10 shares, cost basis $1000
		testutil.CreateTestInvestment(t, db, account.ID, sec.ID)

		// Create security price: $150.00 per share
		testutil.CreateTestSecurityPrice(t, db, sec.ID, 15000, time.Now())

		result, err := svc.GetAccountByID(user.ID, account.ID)
		testutil.AssertNoError(t, err)

		// Expected balance = 10 shares * $150.00 = $1500.00 = 150000 cents
		if result.Balance != 150000 {
			t.Errorf("expected balance 150000, got %d", result.Balance)
		}
	})

	t.Run("leaves_cash_account_unchanged", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 5000)

		result, err := svc.GetAccountByID(user.ID, account.ID)
		testutil.AssertNoError(t, err)

		if result.Balance != 5000 {
			t.Errorf("expected cash balance 5000, got %d", result.Balance)
		}
	})
}

func TestUpdateAccountBalance_CreditCard(t *testing.T) {
	t.Run("expense_increases_credit_card_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 0)

		err := svc.UpdateAccountBalance(db, account, models.TransactionTypeExpense, 5000)
		testutil.AssertNoError(t, err)

		if account.Balance != 5000 {
			t.Errorf("expected balance 5000 after expense, got %d", account.Balance)
		}

		var dbAccount models.Account
		db.First(&dbAccount, account.ID)
		if dbAccount.Balance != 5000 {
			t.Errorf("expected DB balance 5000, got %d", dbAccount.Balance)
		}
	})

	t.Run("income_decreases_credit_card_balance", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCreditCardAccount(t, db, user.ID, 5000)

		err := svc.UpdateAccountBalance(db, account, models.TransactionTypeIncome, 3000)
		testutil.AssertNoError(t, err)

		if account.Balance != 2000 {
			t.Errorf("expected balance 2000 after payment, got %d", account.Balance)
		}

		var dbAccount models.Account
		db.First(&dbAccount, account.ID)
		if dbAccount.Balance != 2000 {
			t.Errorf("expected DB balance 2000, got %d", dbAccount.Balance)
		}
	})

	t.Run("cash_account_unchanged_behavior", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewAccountService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000)

		err := svc.UpdateAccountBalance(db, account, models.TransactionTypeIncome, 500)
		testutil.AssertNoError(t, err)

		if account.Balance != 1500 {
			t.Errorf("expected balance 1500, got %d", account.Balance)
		}
	})
}
