package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateBudget(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		budget, err := svc.CreateBudget(user.ID, cat.ID, "Groceries", 50000, models.BudgetPeriodMonthly, time.Now(), nil)
		testutil.AssertNoError(t, err)

		if budget.ID == "" {
			t.Fatal("expected non-empty budget ID")
		}
		if budget.Name != "Groceries" {
			t.Errorf("expected name Groceries, got %s", budget.Name)
		}
		if budget.Amount != 50000 {
			t.Errorf("expected amount 50000, got %d", budget.Amount)
		}
		if budget.Period != models.BudgetPeriodMonthly {
			t.Errorf("expected period monthly, got %s", budget.Period)
		}
		if !budget.IsActive {
			t.Error("expected budget to be active")
		}
	})

	t.Run("with_end_date", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		endDate := time.Now().AddDate(0, 6, 0)
		budget, err := svc.CreateBudget(user.ID, cat.ID, "Half Year", 100000, models.BudgetPeriodYearly, time.Now(), &endDate)
		testutil.AssertNoError(t, err)

		if budget.EndDate == nil {
			t.Fatal("expected end date to be set")
		}
	})

	t.Run("invalid_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateBudget(user.ID, "9999", "Bad", 50000, models.BudgetPeriodMonthly, time.Now(), nil)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("wrong_user_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user2.ID, models.CategoryTypeExpense)

		_, err := svc.CreateBudget(user1.ID, cat.ID, "Not Mine", 50000, models.BudgetPeriodMonthly, time.Now(), nil)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})
}

func TestGetUserBudgets(t *testing.T) {
	t.Run("returns_user_budgets_only", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat1 := testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)
		cat2 := testutil.CreateTestCategory(t, db, user2.ID, models.CategoryTypeExpense)

		testutil.CreateTestBudget(t, db, user1.ID, cat1.ID)
		testutil.CreateTestBudget(t, db, user1.ID, cat1.ID)
		testutil.CreateTestBudget(t, db, user2.ID, cat2.ID)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserBudgets(user1.ID, page, nil, nil)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 budgets, got %d", result.TotalItems)
		}
	})

	t.Run("filter_by_is_active", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		testutil.CreateTestBudget(t, db, user.ID, cat.ID) // active by default
		// Create a budget then deactivate it (GORM ignores false for default:true on create)
		inactiveBudget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)
		if err := db.Model(inactiveBudget).Update("is_active", false).Error; err != nil {
			t.Fatalf("failed to deactivate budget: %v", err)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		active := true
		result, err := svc.GetUserBudgets(user.ID, page, &active, nil)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 active budget, got %d", result.TotalItems)
		}
	})

	t.Run("filter_by_period", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		testutil.CreateTestBudget(t, db, user.ID, cat.ID) // monthly by default
		// Create a yearly budget directly
		yearlyBudget := &models.Budget{
			UserID:     user.ID,
			CategoryID: cat.ID,
			Name:       "Yearly",
			Amount:     120000,
			Period:     models.BudgetPeriodYearly,
			StartDate:  time.Now(),
			IsActive:   true,
		}
		if err := db.Create(yearlyBudget).Error; err != nil {
			t.Fatalf("failed to create yearly budget: %v", err)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		period := models.BudgetPeriodYearly
		result, err := svc.GetUserBudgets(user.ID, page, nil, &period)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 1 {
			t.Errorf("expected 1 yearly budget, got %d", result.TotalItems)
		}
		if len(result.Data) > 0 && result.Data[0].Period != models.BudgetPeriodYearly {
			t.Errorf("expected yearly period, got %s", result.Data[0].Period)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		for i := 0; i < 5; i++ {
			testutil.CreateTestBudget(t, db, user.ID, cat.ID)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := svc.GetUserBudgets(user.ID, page, nil, nil)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected 5 total items, got %d", result.TotalItems)
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page, got %d", len(result.Data))
		}
	})
}

func TestGetBudgetByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		found, err := svc.GetBudgetByID(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if found.ID != budget.ID {
			t.Errorf("expected budget ID %d, got %d", budget.ID, found.ID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetBudgetByID(user.ID, "9999")
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user1.ID, cat.ID)

		_, err := svc.GetBudgetByID(user2.ID, budget.ID)
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})
}

func TestUpdateBudget(t *testing.T) {
	t.Run("update_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		updated, err := svc.UpdateBudget(user.ID, budget.ID, "New Name", nil, nil, nil)
		testutil.AssertNoError(t, err)

		if updated.Name != "New Name" {
			t.Errorf("expected name 'New Name', got %s", updated.Name)
		}
	})

	t.Run("update_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		newAmount := int64(75000)
		updated, err := svc.UpdateBudget(user.ID, budget.ID, "", &newAmount, nil, nil)
		testutil.AssertNoError(t, err)

		// Re-fetch to verify DB
		fetched, err := svc.GetBudgetByID(user.ID, updated.ID)
		testutil.AssertNoError(t, err)
		if fetched.Amount != 75000 {
			t.Errorf("expected amount 75000, got %d", fetched.Amount)
		}
	})

	t.Run("update_period", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // monthly

		newPeriod := models.BudgetPeriodYearly
		updated, err := svc.UpdateBudget(user.ID, budget.ID, "", nil, &newPeriod, nil)
		testutil.AssertNoError(t, err)

		fetched, err := svc.GetBudgetByID(user.ID, updated.ID)
		testutil.AssertNoError(t, err)
		if fetched.Period != models.BudgetPeriodYearly {
			t.Errorf("expected period yearly, got %s", fetched.Period)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.UpdateBudget(user.ID, "9999", "Nope", nil, nil, nil)
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})
}

func TestDeleteBudget(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		err := svc.DeleteBudget(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		// Should not be findable after soft delete
		_, err = svc.GetBudgetByID(user.ID, budget.ID)
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")

		// Verify it's a soft delete (record exists with deleted_at set)
		var count int64
		db.Unscoped().Model(&models.Budget{}).Where("id = ?", budget.ID).Count(&count)
		if count != 1 {
			t.Errorf("expected soft-deleted record to exist, count=%d", count)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		err := svc.DeleteBudget(user.ID, "9999")
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user1.ID, cat.ID)

		err := svc.DeleteBudget(user2.ID, budget.ID)
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})
}

func TestGetBudgetProgress(t *testing.T) {
	t.Run("no_spending", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // $100

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if progress.BudgetID != budget.ID {
			t.Errorf("expected budget ID %d, got %d", budget.ID, progress.BudgetID)
		}
		if progress.Budgeted != 10000 {
			t.Errorf("expected budgeted 10000, got %d", progress.Budgeted)
		}
		if progress.Spent != 0 {
			t.Errorf("expected spent 0, got %d", progress.Spent)
		}
		if progress.Remaining != 10000 {
			t.Errorf("expected remaining 10000, got %d", progress.Remaining)
		}
		if progress.Percentage != 0 {
			t.Errorf("expected percentage 0, got %f", progress.Percentage)
		}
	})

	t.Run("partial_spending", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // $100

		// Create expense transactions with the budget's category in the current month
		catID := cat.ID
		tx1 := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &catID,
			Type:       models.TransactionTypeExpense,
			Amount:     3000, // $30
			Date:       time.Now(),
		}
		tx2 := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &catID,
			Type:       models.TransactionTypeExpense,
			Amount:     2000, // $20
			Date:       time.Now(),
		}
		if err := db.Create(tx1).Error; err != nil {
			t.Fatalf("failed to create tx1: %v", err)
		}
		if err := db.Create(tx2).Error; err != nil {
			t.Fatalf("failed to create tx2: %v", err)
		}

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if progress.Spent != 5000 {
			t.Errorf("expected spent 5000, got %d", progress.Spent)
		}
		if progress.Remaining != 5000 {
			t.Errorf("expected remaining 5000, got %d", progress.Remaining)
		}
		if progress.Percentage != 50.0 {
			t.Errorf("expected percentage 50.0, got %f", progress.Percentage)
		}
	})

	t.Run("over_budget", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 200000)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // $100

		catID := cat.ID
		tx := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &catID,
			Type:       models.TransactionTypeExpense,
			Amount:     15000, // $150 (over $100 budget)
			Date:       time.Now(),
		}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if progress.Spent != 15000 {
			t.Errorf("expected spent 15000, got %d", progress.Spent)
		}
		if progress.Remaining != -5000 {
			t.Errorf("expected remaining -5000, got %d", progress.Remaining)
		}
		if progress.Percentage != 150.0 {
			t.Errorf("expected percentage 150.0, got %f", progress.Percentage)
		}
	})

	t.Run("ignores_income_transactions", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		catID := cat.ID
		// Income tx should not count toward budget progress
		incomeTx := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &catID,
			Type:       models.TransactionTypeIncome,
			Amount:     5000,
			Date:       time.Now(),
		}
		if err := db.Create(incomeTx).Error; err != nil {
			t.Fatalf("failed to create income tx: %v", err)
		}

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if progress.Spent != 0 {
			t.Errorf("expected spent 0 (income should be ignored), got %d", progress.Spent)
		}
	})

	t.Run("ignores_other_category_expenses", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat1 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		cat2 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat1.ID) // budget for cat1

		cat2ID := cat2.ID
		// Expense for different category should not count
		tx := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &cat2ID,
			Type:       models.TransactionTypeExpense,
			Amount:     5000,
			Date:       time.Now(),
		}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		if progress.Spent != 0 {
			t.Errorf("expected spent 0 (different category), got %d", progress.Spent)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetBudgetProgress(user.ID, "9999")
		testutil.AssertAppError(t, err, "BUDGET_NOT_FOUND")
	})

	t.Run("zero_budget_amount", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		// Create budget with zero amount
		budget, err := svc.CreateBudget(user.ID, cat.ID, "Zero", 0, models.BudgetPeriodMonthly, time.Now(), nil)
		testutil.AssertNoError(t, err)

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		// Should not panic with divide-by-zero
		if progress.Percentage != 0 {
			t.Errorf("expected percentage 0 for zero budget, got %f", progress.Percentage)
		}
	})
}
