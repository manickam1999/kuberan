package services

import (
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// descContains builds a single "description contains <text>" condition input.
func descContains(text string) RuleConditionInput {
	return RuleConditionInput{Field: models.RuleFieldDescription, Operator: models.RuleOpContains, ValueText: text}
}

// setCat builds a single set_category action input.
func setCat(catID string) RuleActionInput {
	return RuleActionInput{ActionType: models.RuleActionSetCategory, CategoryID: &catID}
}

func TestRuleService_CreateAndGet(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{
		Name:       "Grab",
		Conditions: []RuleConditionInput{descContains("grab")},
		Actions:    []RuleActionInput{setCat(cat.ID)},
	})
	testutil.AssertNoError(t, err)

	if rule.ID == "" {
		t.Fatal("expected rule ID")
	}
	if len(rule.Conditions) != 1 || rule.Conditions[0].ValueText != "grab" {
		t.Errorf("expected one condition 'grab', got %+v", rule.Conditions)
	}
	if len(rule.Actions) != 1 || rule.Actions[0].CategoryID == nil || *rule.Actions[0].CategoryID != cat.ID {
		t.Errorf("expected one set_category action for %s, got %+v", cat.ID, rule.Actions)
	}
	if !rule.IsActive {
		t.Error("expected rule active by default")
	}
}

func TestRuleService_CreateValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	expCat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)
	incCat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeIncome)

	tests := []struct {
		name string
		in   CreateRuleInput
		code string
	}{
		{
			name: "no conditions",
			in:   CreateRuleInput{Name: "x", Actions: []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_INVALID",
		},
		{
			name: "no actions",
			in:   CreateRuleInput{Name: "x", Conditions: []RuleConditionInput{descContains("grab")}},
			code: "RULE_INVALID",
		},
		{
			name: "description without value",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldDescription, Operator: models.RuleOpContains}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_CONDITION_INVALID",
		},
		{
			name: "between min>max",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldAmount, Operator: models.RuleOpBetween, AmountMin: i64Ptr(9000), AmountMax: i64Ptr(1000)}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_CONDITION_INVALID",
		},
		{
			name: "amount gt without min",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldAmount, Operator: models.RuleOpGt}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_CONDITION_INVALID",
		},
		{
			name: "type invalid value",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldType, Operator: models.RuleOpEquals, ValueText: "transfer"}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_CONDITION_INVALID",
		},
		{
			name: "account not owned",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldAccountID, Operator: models.RuleOpEquals, ValueText: "nonexistent"}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}},
			code: "RULE_CONDITION_INVALID",
		},
		{
			name: "action category not found",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{descContains("grab")},
				Actions:    []RuleActionInput{setCat("missing")}},
			code: "CATEGORY_NOT_FOUND",
		},
		{
			name: "unsupported action type",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{descContains("grab")},
				Actions:    []RuleActionInput{{ActionType: models.RuleActionType("add_tag"), CategoryID: &expCat.ID}}},
			code: "RULE_ACTION_INVALID",
		},
		{
			name: "type condition mismatches category type",
			in: CreateRuleInput{Name: "x",
				Conditions: []RuleConditionInput{{Field: models.RuleFieldType, Operator: models.RuleOpEquals, ValueText: "income"}},
				Actions:    []RuleActionInput{setCat(expCat.ID)}}, // expense cat, income condition
			code: "RULE_CATEGORY_TYPE_MISMATCH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ruleSvc.CreateRule(user.ID, tt.in)
			testutil.AssertAppError(t, err, tt.code)
		})
	}

	// Sanity: a type=income condition with an income category is accepted.
	_, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "salary",
		Conditions: []RuleConditionInput{{Field: models.RuleFieldType, Operator: models.RuleOpEquals, ValueText: "income"}},
		Actions:    []RuleActionInput{setCat(incCat.ID)}})
	testutil.AssertNoError(t, err)
}

func TestRuleService_ListOrderedByPriority(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	mk := func(name string, priority int) {
		_, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: name, Priority: priority,
			Conditions: []RuleConditionInput{descContains(name)}, Actions: []RuleActionInput{setCat(cat.ID)}})
		testutil.AssertNoError(t, err)
	}
	mk("c", 2)
	mk("a", 0)
	mk("b", 1)

	rules, err := ruleSvc.ListRules(user.ID)
	testutil.AssertNoError(t, err)
	got := []string{rules[0].Name, rules[1].Name, rules[2].Name}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order mismatch: got %v, want %v", got, want)
			break
		}
	}
}

func TestRuleService_UpdateReplacesChildren(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "r",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)

	newConds := []RuleConditionInput{descContains("starbucks"), {Field: models.RuleFieldAmount, Operator: models.RuleOpLt, AmountMax: i64Ptr(5000)}}
	inactive := false
	updated, err := ruleSvc.UpdateRule(user.ID, rule.ID, UpdateRuleInput{Conditions: newConds, IsActive: &inactive})
	testutil.AssertNoError(t, err)

	if len(updated.Conditions) != 2 {
		t.Fatalf("expected 2 conditions after replace, got %d", len(updated.Conditions))
	}
	if updated.IsActive {
		t.Error("expected rule to be inactive after update")
	}

	// Confirm the old condition rows were hard-deleted, not accumulated.
	var count int64
	db.Unscoped().Model(&models.TransactionRuleCondition{}).Where("rule_id = ?", rule.ID).Count(&count)
	if count != 2 {
		t.Errorf("expected exactly 2 condition rows (old hard-deleted), got %d", count)
	}
}

func TestRuleService_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "r",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)

	testutil.AssertNoError(t, ruleSvc.DeleteRule(user.ID, rule.ID))
	_, err = ruleSvc.GetRule(user.ID, rule.ID)
	testutil.AssertAppError(t, err, "RULE_NOT_FOUND")
}

func TestRuleService_Reorder(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	var ids []string
	for _, n := range []string{"a", "b", "c"} {
		r, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: n,
			Conditions: []RuleConditionInput{descContains(n)}, Actions: []RuleActionInput{setCat(cat.ID)}})
		testutil.AssertNoError(t, err)
		ids = append(ids, r.ID)
	}

	// Reverse the order.
	reordered, err := ruleSvc.ReorderRules(user.ID, []string{ids[2], ids[1], ids[0]})
	testutil.AssertNoError(t, err)
	if reordered[0].ID != ids[2] || reordered[2].ID != ids[0] {
		t.Errorf("expected reversed order, got %s..%s", reordered[0].ID, reordered[2].ID)
	}
	if reordered[0].Priority != 0 {
		t.Errorf("expected first rule priority 0, got %d", reordered[0].Priority)
	}
}

func TestRuleService_ResolveForUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	expCat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	_, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(expCat.ID)}})
	testutil.AssertNoError(t, err)

	t.Run("matches expense", func(t *testing.T) {
		res, err := ruleSvc.ResolveForUser(user.ID, RuleInput{Description: "GRAB RIDE", Amount: 2000, Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if res.CategoryID == nil || *res.CategoryID != expCat.ID {
			t.Errorf("expected category %s, got %v", expCat.ID, res.CategoryID)
		}
	})

	t.Run("income input does not get expense category", func(t *testing.T) {
		res, err := ruleSvc.ResolveForUser(user.ID, RuleInput{Description: "GRAB RIDE", Amount: 2000, Type: models.TransactionTypeIncome})
		testutil.AssertNoError(t, err)
		if res.CategoryID != nil {
			t.Errorf("expected no category for type-mismatched input, got %v", *res.CategoryID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		res, err := ruleSvc.ResolveForUser(user.ID, RuleInput{Description: "STARBUCKS", Amount: 2000, Type: models.TransactionTypeExpense})
		testutil.AssertNoError(t, err)
		if res.CategoryID != nil {
			t.Errorf("expected no match, got %v", *res.CategoryID)
		}
	})
}

func TestCategoryDeleteDeactivatesRules(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	catSvc := NewCategoryService(db)
	ruleSvc := NewRuleService(db, catSvc)
	user := testutil.CreateTestUser(t, db)
	cat := testutil.CreateTestCategory(t, db, user.ID, models.CategoryTypeExpense)

	rule, err := ruleSvc.CreateRule(user.ID, CreateRuleInput{Name: "grab",
		Conditions: []RuleConditionInput{descContains("grab")}, Actions: []RuleActionInput{setCat(cat.ID)}})
	testutil.AssertNoError(t, err)

	testutil.AssertNoError(t, catSvc.DeleteCategory(user.ID, cat.ID))

	reloaded, err := ruleSvc.GetRule(user.ID, rule.ID)
	testutil.AssertNoError(t, err)
	if reloaded.IsActive {
		t.Error("expected rule targeting a deleted category to be deactivated")
	}
}
