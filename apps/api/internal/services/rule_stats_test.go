package services

import (
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

func TestRuleStats_CountsCreateAndBackfill(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	acctSvc := NewAccountService(db)
	ruleSvc := NewRuleService(db, NewCategoryService(db))
	txSvc := NewTransactionService(db, acctSvc, ruleSvc)

	user := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 100000)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	// Two uncategorized historic rows for the backfill.
	for _, desc := range []string{"GRAB OLD 1", "GRAB OLD 2"} {
		tx := testutil.CreateTestTransaction(t, db, user.ID, account.ID, models.TransactionTypeExpense, 1000)
		db.Model(tx).Update("description", desc)
	}

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)
	if rule.MatchCount != 0 || rule.LastMatchedAt != nil {
		t.Fatalf("new rule should have no stats, got %d %v", rule.MatchCount, rule.LastMatchedAt)
	}

	// A matching create counts once; an explicit category and a non-match do not.
	_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 500, "GRAB RIDE", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	_, err = txSvc.CreateTransaction(user.ID, account.ID, &cat.ID, models.TransactionTypeExpense, 500, "GRAB EXPLICIT", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)
	_, err = txSvc.CreateTransaction(user.ID, account.ID, nil, models.TransactionTypeExpense, 500, "STARBUCKS", time.Now(), TransactionMeta{})
	testutil.AssertNoError(t, err)

	got, err := ruleSvc.GetRule(user.ID, rule.ID)
	testutil.AssertNoError(t, err)
	if got.MatchCount != 1 || got.LastMatchedAt == nil {
		t.Fatalf("expected 1 match with a timestamp, got %d %v", got.MatchCount, got.LastMatchedAt)
	}

	// A dry run changes nothing; the real backfill adds the rows it changed.
	_, err = txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized, DryRun: true})
	testutil.AssertNoError(t, err)
	got, _ = ruleSvc.GetRule(user.ID, rule.ID)
	if got.MatchCount != 1 {
		t.Errorf("dry run must not count, got %d", got.MatchCount)
	}
	res, err := txSvc.ApplyRule(user.ID, rule.ID, ApplyRuleOptions{Scope: RuleApplyScopeUncategorized})
	testutil.AssertNoError(t, err)
	if res.Applied != 2 {
		t.Fatalf("expected 2 applied, got %d", res.Applied)
	}
	got, _ = ruleSvc.GetRule(user.ID, rule.ID)
	if got.MatchCount != 3 {
		t.Errorf("expected 3 matches after backfill, got %d", got.MatchCount)
	}
}

func TestRuleService_SuggestRules(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	ruleSvc := NewRuleService(db, NewCategoryService(db))

	user := testutil.CreateTestUser(t, db)
	other := testutil.CreateTestUser(t, db)
	account := testutil.CreateTestCashAccount(t, db, user.ID)
	otherAccount := testutil.CreateTestCashAccount(t, db, other.ID)
	food := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	fuel := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	covered := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	otherCat := testutil.CreateTestCategory(t, db, other.ID, models.CategoryTypeExpense)

	add := func(userID, accountID, categoryID, desc string, ago time.Duration) {
		tx := testutil.CreateTestTransaction(t, db, userID, accountID, models.TransactionTypeExpense, 1000)
		testutil.AssertNoError(t, db.Model(tx).Updates(map[string]any{
			"description": desc, "category_id": categoryID, "date": time.Now().Add(-ago),
		}).Error)
	}
	day := 24 * time.Hour
	// Qualifies: 3 rows, case/whitespace differences normalise together.
	add(user.ID, account.ID, food.ID, "Kopi  Tiam", day)
	add(user.ID, account.ID, food.ID, "kopi tiam", 2*day)
	add(user.ID, account.ID, food.ID, "KOPI TIAM", 3*day)
	// Only 2 rows: no suggestion.
	add(user.ID, account.ID, fuel.ID, "petron", day)
	add(user.ID, account.ID, fuel.ID, "petron", day)
	// Same description in two categories: neither reaches 3.
	add(user.ID, account.ID, food.ID, "mixed", day)
	add(user.ID, account.ID, food.ID, "mixed", day)
	add(user.ID, account.ID, fuel.ID, "mixed", day)
	// Outside the 90-day window.
	for i := 0; i < 3; i++ {
		add(user.ID, account.ID, fuel.ID, "old shop", 100*day)
	}
	// Covered by an existing rule.
	for i := 0; i < 3; i++ {
		add(user.ID, account.ID, covered.ID, "grab ride", day)
	}
	_, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(covered.ID)}})
	testutil.AssertNoError(t, err)
	// Another user's repeats are never visible.
	for i := 0; i < 5; i++ {
		add(other.ID, otherAccount.ID, otherCat.ID, "secret shop", day)
	}

	got, err := ruleSvc.SuggestRules(user.ID)
	testutil.AssertNoError(t, err)
	if len(got) != 1 {
		t.Fatalf("expected 1 suggestion, got %+v", got)
	}
	s := got[0]
	if s.DescriptionPattern != "kopi tiam" || s.CategoryID != food.ID || s.Count != 3 || s.Type != models.TransactionTypeExpense {
		t.Errorf("unexpected suggestion %+v", s)
	}

	// A paused rule does not cover its pattern.
	paused := false
	pausedRule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "kopi",
		Conditions: []RuleConditionInput{descContains("kopi")}, Actions: []RuleActionInput{setCat(food.ID)}})
	testutil.AssertNoError(t, err)
	_, err = ruleSvc.UpdateRule(user.ID, pausedRule.ID, UpdateRuleInput{IsActive: &paused})
	testutil.AssertNoError(t, err)
	got, _ = ruleSvc.SuggestRules(user.ID)
	if len(got) != 1 {
		t.Errorf("paused rule should not suppress the suggestion, got %+v", got)
	}
	_, err = ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "kopi2",
		Conditions: []RuleConditionInput{descContains("kopi")}, Actions: []RuleActionInput{setCat(food.ID)}})
	testutil.AssertNoError(t, err)
	got, _ = ruleSvc.SuggestRules(user.ID)
	if len(got) != 0 {
		t.Errorf("active rule should suppress the suggestion, got %+v", got)
	}
}
