package services

import (
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

func TestCreateCategory(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		cat, err := svc.CreateCategory(user.ID, "Groceries", models.CategoryTypeExpense, "Food shopping", "cart", "#FF0000", nil)
		testutil.AssertNoError(t, err)

		if cat.ID == "" {
			t.Fatal("expected non-empty category ID")
		}
		if cat.Name != "Groceries" {
			t.Errorf("expected name Groceries, got %s", cat.Name)
		}
		if cat.Type != models.CategoryTypeExpense {
			t.Errorf("expected type expense, got %s", cat.Type)
		}
		if cat.Description != "Food shopping" {
			t.Errorf("expected description 'Food shopping', got %s", cat.Description)
		}
	})

	t.Run("duplicate_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCategory(user.ID, "Food", models.CategoryTypeExpense, "", "", "", nil)
		testutil.AssertNoError(t, err)

		_, err = svc.CreateCategory(user.ID, "Food", models.CategoryTypeExpense, "", "", "", nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("with_parent", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		parent, err := svc.CreateCategory(user.ID, "Food", models.CategoryTypeExpense, "", "", "", nil)
		testutil.AssertNoError(t, err)

		child, err := svc.CreateCategory(user.ID, "Snacks", models.CategoryTypeExpense, "", "", "", &parent.ID)
		testutil.AssertNoError(t, err)

		if child.ParentID == nil || *child.ParentID != parent.ID {
			t.Errorf("expected parent ID %d, got %v", parent.ID, child.ParentID)
		}
	})

	t.Run("invalid_parent", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		nonexistent := uint(99999)
		_, err := svc.CreateCategory(user.ID, "Orphan", models.CategoryTypeExpense, "", "", "", &nonexistent)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("empty_name", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCategory(user.ID, "", models.CategoryTypeExpense, "", "", "", nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("duplicate_name_different_users_allowed", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCategory(user1.ID, "Salary", models.CategoryTypeIncome, "", "", "", nil)
		testutil.AssertNoError(t, err)

		// Same name for different user should succeed
		_, err = svc.CreateCategory(user2.ID, "Salary", models.CategoryTypeIncome, "", "", "", nil)
		testutil.AssertNoError(t, err)
	})
}

func TestGetUserCategories(t *testing.T) {
	t.Run("returns_user_categories_only", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)

		testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)
		testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeIncome)
		testutil.CreateTestCategory(t, db, user2.ID, models.CategoryTypeExpense)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserCategories(user1.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 categories for user1, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 categories in data, got %d", len(result.Data))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		for i := 0; i < 5; i++ {
			testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		}

		page := pagination.PageRequest{Page: 1, PageSize: 2}
		result, err := svc.GetUserCategories(user.ID, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 5 {
			t.Errorf("expected 5 total items, got %d", result.TotalItems)
		}
		if len(result.Data) != 2 {
			t.Errorf("expected 2 items on page 1, got %d", len(result.Data))
		}
		if result.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", result.TotalPages)
		}
	})
}

func TestGetUserCategoriesByType(t *testing.T) {
	t.Run("filters_correctly", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)

		page := pagination.PageRequest{Page: 1, PageSize: 20}
		result, err := svc.GetUserCategoriesByType(user.ID, models.CategoryTypeExpense, page)
		testutil.AssertNoError(t, err)

		if result.TotalItems != 2 {
			t.Errorf("expected 2 expense categories, got %d", result.TotalItems)
		}

		for _, cat := range result.Data {
			if cat.Type != models.CategoryTypeExpense {
				t.Errorf("expected type expense, got %s", cat.Type)
			}
		}
	})
}

func TestGetCategoryByID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		created := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		cat, err := svc.GetCategoryByID(user.ID, created.ID)
		testutil.AssertNoError(t, err)

		if cat.ID != created.ID {
			t.Errorf("expected category ID %d, got %d", created.ID, cat.ID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.GetCategoryByID(user.ID, 99999)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)

		_, err := svc.GetCategoryByID(user2.ID, cat.ID)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})
}

func TestUpdateCategory(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		updated, err := svc.UpdateCategory(user.ID, cat.ID, "New Name", "New Desc", "star", "#00FF00", nil)
		testutil.AssertNoError(t, err)

		if updated.Name != "New Name" {
			t.Errorf("expected name 'New Name', got %s", updated.Name)
		}
		if updated.Description != "New Desc" {
			t.Errorf("expected description 'New Desc', got %s", updated.Description)
		}
		if updated.Icon != "star" {
			t.Errorf("expected icon 'star', got %s", updated.Icon)
		}
		if updated.Color != "#00FF00" {
			t.Errorf("expected color '#00FF00', got %s", updated.Color)
		}
	})

	t.Run("self_parent", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := svc.UpdateCategory(user.ID, cat.ID, "", "", "", "", &cat.ID)
		testutil.AssertAppError(t, err, "SELF_PARENT_CATEGORY")
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.UpdateCategory(user.ID, 99999, "Name", "", "", "", nil)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("with_valid_parent", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		parent := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		child := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		updated, err := svc.UpdateCategory(user.ID, child.ID, "", "", "", "", &parent.ID)
		testutil.AssertNoError(t, err)

		if updated.ParentID == nil || *updated.ParentID != parent.ID {
			t.Errorf("expected parent ID %d, got %v", parent.ID, updated.ParentID)
		}
	})
}

func TestDeleteCategory(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		err := svc.DeleteCategory(user.ID, cat.ID)
		testutil.AssertNoError(t, err)

		// Verify soft-deleted (not found via service)
		_, err = svc.GetCategoryByID(user.ID, cat.ID)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")

		// Verify still exists in DB with deleted_at set (soft delete)
		var count int64
		db.Unscoped().Model(&models.Category{}).Where("id = ?", cat.ID).Count(&count)
		if count != 1 {
			t.Errorf("expected soft-deleted record to exist in DB, got count %d", count)
		}
	})

	t.Run("has_children", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		parent := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		// Create child directly via DB with parent_id
		child := &models.Category{
			UserID:   user.ID,
			Name:     "Child Category",
			Type:     models.CategoryTypeExpense,
			ParentID: &parent.ID,
		}
		if err := db.Create(child).Error; err != nil {
			t.Fatalf("failed to create child category: %v", err)
		}

		err := svc.DeleteCategory(user.ID, parent.ID)
		testutil.AssertAppError(t, err, "CATEGORY_HAS_CHILDREN")
	})

	t.Run("allows_deletion_when_transactions_reference_category", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		account := testutil.CreateTestCashAccount(t, db, user.ID)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		// Create a transaction referencing this category
		tx := &models.Transaction{
			UserID:     user.ID,
			AccountID:  account.ID,
			CategoryID: &cat.ID,
			Type:       models.TransactionTypeExpense,
			Amount:     1000,
		}
		if err := db.Create(tx).Error; err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}

		// Should succeed (soft delete allowed even with referencing transactions)
		err := svc.DeleteCategory(user.ID, cat.ID)
		testutil.AssertNoError(t, err)

		// Transaction should still reference the soft-deleted category
		var storedTx models.Transaction
		db.First(&storedTx, tx.ID)
		if storedTx.CategoryID == nil || *storedTx.CategoryID != cat.ID {
			t.Error("expected transaction to still reference the soft-deleted category")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		err := svc.DeleteCategory(user.ID, 99999)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})

	t.Run("wrong_user", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)

		user1 := testutil.CreateTestUser(t, db)
		user2 := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user1.ID, models.CategoryTypeExpense)

		err := svc.DeleteCategory(user2.ID, cat.ID)
		testutil.AssertAppError(t, err, "CATEGORY_NOT_FOUND")
	})
}
