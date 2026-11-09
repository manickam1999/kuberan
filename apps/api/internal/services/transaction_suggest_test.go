package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestSuggestCategory(t *testing.T) {
	e := newRecEnv(t)
	user := e.user.ID
	dining := testutil.CreateTestCategory(t, e.db, user, models.CategoryTypeExpense)
	transport := testutil.CreateTestCategory(t, e.db, user, models.CategoryTypeExpense)

	create := func(desc string, cat *string, when time.Time) {
		t.Helper()
		_, err := e.txn.CreateTransaction(user, e.cash.ID, cat, models.TransactionTypeExpense, 500, desc, when, TransactionMeta{})
		testutil.AssertNoError(t, err)
	}

	t.Run("nothing applies", func(t *testing.T) {
		got, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "coffee", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("history picks the most recent category, normalising the description", func(t *testing.T) {
		create("Coffee", &dining.ID, time.Now().Add(-48*time.Hour))
		create("coffee", &transport.ID, time.Now().Add(-24*time.Hour))
		got, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "  COFFEE ", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got == nil || got.CategoryID != transport.ID || got.Reason != SuggestReasonHistory || got.CategoryName != transport.Name {
			t.Fatalf("unexpected suggestion: %+v", got)
		}
		// A different type does not share history.
		got, err = e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "coffee", Type: models.TransactionTypeIncome})
		testutil.AssertNoError(t, err)
		if got != nil {
			t.Fatalf("income must not use expense history, got %+v", got)
		}
	})

	t.Run("a rule wins over history", func(t *testing.T) {
		_, err := e.rules.CreateRule(user, CreateRuleInput{
			Name:       "coffee",
			Conditions: []RuleConditionInput{descContains("coffee")},
			Actions:    []RuleActionInput{setCat(dining.ID)},
		})
		testutil.AssertNoError(t, err)
		got, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "coffee", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got == nil || got.CategoryID != dining.ID || got.Reason != SuggestReasonRule {
			t.Fatalf("unexpected suggestion: %+v", got)
		}
	})

	t.Run("deleted category is not suggested", func(t *testing.T) {
		create("bus", &transport.ID, time.Now())
		testutil.AssertNoError(t, e.db.Delete(transport).Error)
		got, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "bus", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("is scoped to the user", func(t *testing.T) {
		other := testutil.CreateTestUser(t, e.db)
		got, err := e.txn.SuggestCategory(other.ID, CategorySuggestionInput{Description: "coffee", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got != nil {
			t.Fatalf("another user's rules/history leaked: %+v", got)
		}
	})

	t.Run("rejects non income/expense types", func(t *testing.T) {
		_, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "x", Type: models.TransactionTypeTransfer})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})

	t.Run("blank description gives nothing", func(t *testing.T) {
		got, err := e.txn.SuggestCategory(user, CategorySuggestionInput{Description: "   ", Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
}
