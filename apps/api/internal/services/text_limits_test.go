package services

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestServicesEnforceTextLimits(t *testing.T) {
	long := func(n int) string { return strings.Repeat("₹", n) } // 3 bytes per character

	tests := []struct {
		name string
		run  func(t *testing.T, env textEnv, okLen int) error
		max  int
	}{
		{"transaction description", func(_ *testing.T, e textEnv, n int) error {
			_, err := e.tx.CreateTransaction(e.user.ID, e.account.ID, nil, models.TransactionTypeExpense, 100, long(n), time.Now())
			return err
		}, MaxDescriptionLen},
		{"transfer description", func(t *testing.T, e textEnv, n int) error {
			to := testutil.CreateTestCashAccount(t, e.db, e.user.ID)
			_, err := e.tx.CreateTransfer(e.user.ID, e.account.ID, to.ID, 100, long(n), time.Now())
			return err
		}, MaxDescriptionLen},
		{"category name", func(_ *testing.T, e textEnv, n int) error {
			_, err := e.cat.CreateCategory(e.user.ID, long(n), models.CategoryTypeExpense, "", "", "", nil)
			return err
		}, MaxNameLen},
		// Category icons have stricter content rules than this length limit
		// (an emoji or a named icon); see category_icons_test.go.
		{"category update description", func(t *testing.T, e textEnv, n int) error {
			c := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
			_, err := e.cat.UpdateCategory(e.user.ID, c.ID, "", long(n), "", "", nil)
			return err
		}, MaxDescriptionLen},
		{"budget name", func(t *testing.T, e textEnv, n int) error {
			c := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
			_, err := e.budget.CreateBudget(e.user.ID, c.ID, long(n), 1000, models.BudgetPeriodMonthly)
			return err
		}, MaxNameLen},
		{"budget rename", func(t *testing.T, e textEnv, n int) error {
			c := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
			b := testutil.CreateTestBudget(t, e.db, e.user.ID, c.ID)
			_, err := e.budget.UpdateBudget(e.user.ID, b.ID, long(n), nil, nil, nil)
			return err
		}, MaxNameLen},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testutil.AssertNoError(t, tc.run(t, newTextEnv(t), tc.max))
			testutil.AssertAppError(t, tc.run(t, newTextEnv(t), tc.max+1), "INVALID_INPUT")
		})
	}
}

type textEnv struct {
	db      *gorm.DB
	user    *models.User
	account *models.Account
	tx      TransactionServicer
	cat     CategoryServicer
	budget  BudgetServicer
}

func newTextEnv(t *testing.T) textEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	user := testutil.CreateTestUser(t, db)
	return textEnv{
		db:      db,
		user:    user,
		account: testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100_000),
		tx:      NewTransactionService(db, NewAccountService(db), nil),
		cat:     NewCategoryService(db),
		budget:  NewBudgetService(db),
	}
}
