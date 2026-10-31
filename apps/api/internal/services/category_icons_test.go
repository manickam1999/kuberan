package services

import (
	"errors"
	"testing"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestValidateCategoryIcon(t *testing.T) {
	tests := []struct {
		name  string
		icon  string
		valid bool
	}{
		{"empty", "", true},
		{"named icon", "shopping-cart", true},
		{"system interest icon", "landmark", true},
		{"emoji", "🍕", true},
		{"emoji with variation selector", "☕️", true},
		{"skin tone modifier", "👍🏽", true},
		{"zwj sequence", "👩‍💻", true},
		{"flag", "🇲🇾", true},
		{"unknown name", "cart", false},
		{"free text", "Food", false},
		{"emoji with text", "🍕 food", false},
		{"two words", "shopping bag", false},
		{"digit", "1", false},
		{"cjk letter", "食", false},
		{"too many emoji", "🍕🍕🍕🍕🍕🍕🍕🍕🍕🍕🍕", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCategoryIcon(tt.icon)
			if tt.valid && err != nil {
				t.Fatalf("expected %q to be valid, got %v", tt.icon, err)
			}
			if !tt.valid && !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("expected %q to be INVALID_INPUT, got %v", tt.icon, err)
			}
		})
	}
}

func TestCategoryIcon_ServiceRules(t *testing.T) {
	t.Run("create rejects free text", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)

		_, err := svc.CreateCategory(user.ID, "Groceries", models.CategoryTypeExpense, "", "groceries", "", nil)
		if !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("expected INVALID_INPUT, got %v", err)
		}
	})

	t.Run("update keeps a legacy icon sent back unchanged", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
		// Saved before icons were restricted.
		if err := db.Model(cat).Update("icon", "food").Error; err != nil {
			t.Fatalf("seed legacy icon: %v", err)
		}

		updated, err := svc.UpdateCategory(user.ID, cat.ID, "Renamed", "", "food", "", nil)
		testutil.AssertNoError(t, err)
		if updated.Name != "Renamed" || updated.Icon != "food" {
			t.Fatalf("expected rename with legacy icon kept, got name=%q icon=%q", updated.Name, updated.Icon)
		}
	})

	t.Run("update rejects a new free-text icon", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		defer testutil.TeardownTestDB(t, db)
		svc := NewCategoryService(db)
		user := testutil.CreateTestUser(t, db)
		cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

		_, err := svc.UpdateCategory(user.ID, cat.ID, "", "", "food", "", nil)
		if !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("expected INVALID_INPUT, got %v", err)
		}
	})
}
