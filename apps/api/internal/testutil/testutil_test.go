package testutil_test

import (
	"testing"

	"kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestSetupTestDB(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	// Verify all tables exist by doing a simple count query on each model.
	var count int64
	for _, table := range []string{"users", "accounts", "categories", "transactions", "budgets", "securities", "security_prices", "portfolio_snapshots", "investments", "investment_transactions", "audit_logs"} {
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Errorf("table %q should exist after migration: %v", table, err)
		}
	}
}

func TestFixtures(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	user := testutil.CreateTestUser(t, db)
	if user.ID == "" {
		t.Fatal("user should have a non-empty ID")
	}

	account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 5000)
	if account.Balance != 5000 {
		t.Errorf("expected balance 5000, got %d", account.Balance)
	}

	invAccount := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	if invAccount.Type != models.AccountTypeInvestment {
		t.Errorf("expected investment account type, got %s", invAccount.Type)
	}

	category := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	if category.Type != models.CategoryTypeExpense {
		t.Errorf("expected expense category, got %s", category.Type)
	}

	tx := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeIncome, 1000)
	if tx.Amount != 1000 {
		t.Errorf("expected amount 1000, got %d", tx.Amount)
	}

	budget := testutil.CreateTestBudget(t, db, user.ID, category.ID)
	if budget.Amount != 10000 {
		t.Errorf("expected budget amount 10000, got %d", budget.Amount)
	}

	sec := testutil.CreateTestSecurity(t, db)
	if sec.ID == "" {
		t.Fatal("security should have a non-empty ID")
	}

	inv := testutil.CreateTestInvestment(t, db, invAccount.ID, sec.ID)
	if inv.Quantity != 10.0 {
		t.Errorf("expected quantity 10.0, got %f", inv.Quantity)
	}
}

func TestAssertAppError(t *testing.T) {
	err := errors.WithMessage(errors.ErrAccountNotFound, "custom message")
	testutil.AssertAppError(t, err, "ACCOUNT_NOT_FOUND")
}

func TestAssertNoError(t *testing.T) {
	testutil.AssertNoError(t, nil)
}
