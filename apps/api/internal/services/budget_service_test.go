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

		budget, err := svc.CreateBudget(user.ID, cat.ID, "Groceries", 50000, models.BudgetPeriodMonthly)
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

	t.Run("invalid_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateBudget(user.ID, "9999", "Bad", 50000, models.BudgetPeriodMonthly)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("wrong_user_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user2.ID, models.CategoryTypeExpense)

		_, err := svc.CreateBudget(user1.ID, cat.ID, "Not Mine", 50000, models.BudgetPeriodMonthly)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("rejects_duplicate_active_budget", func(t *testing.T) {
		// D4: a second active budget for the same (user, category, period) is rejected.
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := svc.CreateBudget(user.ID, cat.ID, "Groceries", 50000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)

		_, err = svc.CreateBudget(user.ID, cat.ID, "Groceries Again", 60000, models.BudgetPeriodMonthly)
		testutil.AssertAppError(t, err, "BUDGET_ALREADY_EXISTS")
	})

	t.Run("allows_same_category_different_period", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := svc.CreateBudget(user.ID, cat.ID, "Monthly", 50000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)

		_, err = svc.CreateBudget(user.ID, cat.ID, "Yearly", 600000, models.BudgetPeriodYearly)
		testutil.AssertNoError(t, err)
	})

	t.Run("allows_new_budget_when_existing_is_inactive", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		first, err := svc.CreateBudget(user.ID, cat.ID, "Groceries", 50000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)

		// Deactivate the first, then a new active one for the same key is allowed.
		inactive := false
		_, err = svc.UpdateBudget(user.ID, first.ID, "", nil, nil, &inactive)
		testutil.AssertNoError(t, err)

		_, err = svc.CreateBudget(user.ID, cat.ID, "Groceries v2", 60000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)
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
			t.Errorf("expected budget ID %s, got %s", budget.ID, found.ID)
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

	t.Run("update_is_active", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // active

		inactive := false
		_, err := svc.UpdateBudget(user.ID, budget.ID, "", nil, nil, &inactive)
		testutil.AssertNoError(t, err)

		fetched, err := svc.GetBudgetByID(user.ID, budget.ID)
		testutil.AssertNoError(t, err)
		if fetched.IsActive {
			t.Error("expected budget to be inactive after update")
		}

		// Re-activate.
		active := true
		_, err = svc.UpdateBudget(user.ID, budget.ID, "", nil, nil, &active)
		testutil.AssertNoError(t, err)
		fetched, err = svc.GetBudgetByID(user.ID, budget.ID)
		testutil.AssertNoError(t, err)
		if !fetched.IsActive {
			t.Error("expected budget to be active after re-activation")
		}
	})

	t.Run("nil_is_active_leaves_unchanged", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID) // active

		_, err := svc.UpdateBudget(user.ID, budget.ID, "Renamed", nil, nil, nil)
		testutil.AssertNoError(t, err)

		fetched, err := svc.GetBudgetByID(user.ID, budget.ID)
		testutil.AssertNoError(t, err)
		if !fetched.IsActive {
			t.Error("expected budget to remain active when is_active is nil")
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

func TestGetUserBudgets_Ordering(t *testing.T) {
	// GetUserBudgets orders by created_at DESC (newest first) deterministically.
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	svc := NewBudgetService(db)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	// Create three budgets with strictly increasing created_at.
	var ids []string
	base := time.Now().Add(-3 * time.Hour)
	for i := 0; i < 3; i++ {
		b := &models.Budget{
			UserID:     user.ID,
			CategoryID: cat.ID,
			Name:       "B",
			Amount:     10000,
			Period:     models.BudgetPeriodMonthly,
			IsActive:   true,
		}
		if err := db.Create(b).Error; err != nil {
			t.Fatalf("failed to create budget: %v", err)
		}
		// Force a distinct, increasing created_at.
		if err := db.Model(b).Update("created_at", base.Add(time.Duration(i)*time.Hour)).Error; err != nil {
			t.Fatalf("failed to set created_at: %v", err)
		}
		ids = append(ids, b.ID)
	}

	page := pagination.PageRequest{Page: 1, PageSize: 20}
	result, err := svc.GetUserBudgets(user.ID, page, nil, nil)
	testutil.AssertNoError(t, err)

	if len(result.Data) != 3 {
		t.Fatalf("expected 3 budgets, got %d", len(result.Data))
	}
	// Newest (last created) first.
	want := []string{ids[2], ids[1], ids[0]}
	for i, id := range want {
		if result.Data[i].ID != id {
			t.Errorf("position %d: expected %s, got %s", i, id, result.Data[i].ID)
		}
	}
}

func TestGetActiveBudgetsProgress(t *testing.T) {
	t.Run("empty_when_no_budgets", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)

		progress, err := svc.GetActiveBudgetsProgress(user.ID)
		testutil.AssertNoError(t, err)
		if len(progress) != 0 {
			t.Errorf("expected empty progress, got %d entries", len(progress))
		}
	})

	t.Run("excludes_inactive_budgets", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat1 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		cat2 := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		active := testutil.CreateTestBudget(t, db, user.ID, cat1.ID)
		inactive := testutil.CreateTestBudget(t, db, user.ID, cat2.ID)
		if err := db.Model(inactive).Update("is_active", false).Error; err != nil {
			t.Fatalf("failed to deactivate budget: %v", err)
		}

		progress, err := svc.GetActiveBudgetsProgress(user.ID)
		testutil.AssertNoError(t, err)
		if len(progress) != 1 {
			t.Fatalf("expected 1 active budget progress, got %d", len(progress))
		}
		if progress[0].BudgetID != active.ID {
			t.Errorf("expected progress for active budget %s, got %s", active.ID, progress[0].BudgetID)
		}
	})

	t.Run("parity_with_single_progress_calls", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 1000000)

		// Two monthly + one yearly budget across distinct categories.
		catA := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catB := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		catC := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		bA, err := svc.CreateBudget(user.ID, catA.ID, "A", 10000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)
		bB, err := svc.CreateBudget(user.ID, catB.ID, "B", 20000, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)
		bC, err := svc.CreateBudget(user.ID, catC.ID, "C", 500000, models.BudgetPeriodYearly)
		testutil.AssertNoError(t, err)

		now := time.Now()
		spend := func(catID string, amount int64) {
			id := catID
			tx := &models.Transaction{
				UserID:     user.ID,
				AccountID:  account.ID,
				CategoryID: &id,
				Type:       models.TransactionTypeExpense,
				Amount:     amount,
				Date:       now,
			}
			if err := db.Create(tx).Error; err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
		}
		spend(catA.ID, 3000)
		spend(catB.ID, 25000) // over budget
		spend(catC.ID, 100000)

		batch, err := svc.GetActiveBudgetsProgress(user.ID)
		testutil.AssertNoError(t, err)

		byID := make(map[string]BudgetProgress, len(batch))
		for _, p := range batch {
			byID[p.BudgetID] = p
		}

		for _, id := range []string{bA.ID, bB.ID, bC.ID} {
			single, err := svc.GetBudgetProgress(user.ID, id)
			testutil.AssertNoError(t, err)
			got, ok := byID[id]
			if !ok {
				t.Fatalf("batch missing progress for budget %s", id)
			}
			if got.Budgeted != single.Budgeted || got.Spent != single.Spent ||
				got.Remaining != single.Remaining || got.Percentage != single.Percentage {
				t.Errorf("budget %s: batch %+v != single %+v", id, got, *single)
			}
		}
	})

	t.Run("inactivated_budget_excluded_from_batch", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewBudgetService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		budget := testutil.CreateTestBudget(t, db, user.ID, cat.ID)

		// Present while active.
		progress, err := svc.GetActiveBudgetsProgress(user.ID)
		testutil.AssertNoError(t, err)
		if len(progress) != 1 {
			t.Fatalf("expected 1 active budget, got %d", len(progress))
		}

		// Pause it via the service, then it disappears from the batch.
		inactive := false
		_, err = svc.UpdateBudget(user.ID, budget.ID, "", nil, nil, &inactive)
		testutil.AssertNoError(t, err)

		progress, err = svc.GetActiveBudgetsProgress(user.ID)
		testutil.AssertNoError(t, err)
		if len(progress) != 0 {
			t.Errorf("expected 0 active budgets after pause, got %d", len(progress))
		}
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
			t.Errorf("expected budget ID %s, got %s", budget.ID, progress.BudgetID)
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
		budget, err := svc.CreateBudget(user.ID, cat.ID, "Zero", 0, models.BudgetPeriodMonthly)
		testutil.AssertNoError(t, err)

		progress, err := svc.GetBudgetProgress(user.ID, budget.ID)
		testutil.AssertNoError(t, err)

		// Should not panic with divide-by-zero
		if progress.Percentage != 0 {
			t.Errorf("expected percentage 0 for zero budget, got %f", progress.Percentage)
		}
	})
}
