package services

import (
	"testing"

	"kuberan/internal/models"
)

// strPtr / i64Ptr are local helpers for building test rules.
func strPtr(s string) *string { return &s }
func i64Ptr(v int64) *int64   { return &v }

// expenseCat / incomeCat build a preloaded category of the given type for an action.
func expenseCat(id string) *models.Category {
	return &models.Category{Base: models.Base{ID: id}, Type: models.CategoryTypeExpense}
}
func incomeCat(id string) *models.Category {
	return &models.Category{Base: models.Base{ID: id}, Type: models.CategoryTypeIncome}
}

// setCategoryAction builds a set_category action with a preloaded category.
func setCategoryAction(catID string, cat *models.Category) models.TransactionRuleAction {
	return models.TransactionRuleAction{
		ActionType: models.RuleActionSetCategory,
		CategoryID: strPtr(catID),
		Category:   cat,
	}
}

// rule assembles a rule from conditions + a single set_category action.
func rule(id string, priority int, cond []models.TransactionRuleCondition, action models.TransactionRuleAction) models.TransactionRule {
	return models.TransactionRule{
		Base:       models.Base{ID: id},
		Priority:   priority,
		IsActive:   true,
		Conditions: cond,
		Actions:    []models.TransactionRuleAction{action},
	}
}

func cond(field models.RuleField, op models.RuleOperator, text string, min, max *int64) models.TransactionRuleCondition {
	return models.TransactionRuleCondition{Field: field, Operator: op, ValueText: text, AmountMin: min, AmountMax: max}
}

func TestMatch_TextOperators(t *testing.T) {
	expenseInput := RuleInput{Description: "GRAB *RIDE 8823 KUALA LUMPUR", Amount: 2500, Type: models.TransactionTypeExpense}

	tests := []struct {
		name     string
		operator models.RuleOperator
		value    string
		want     bool
	}{
		{"contains match", models.RuleOpContains, "grab", true},
		{"contains case-insensitive", models.RuleOpContains, "GrAb", true},
		{"contains no match", models.RuleOpContains, "starbucks", false},
		{"not_contains match", models.RuleOpNotContains, "starbucks", true},
		{"not_contains no match", models.RuleOpNotContains, "grab", false},
		{"equals whole string", models.RuleOpEquals, "grab *ride 8823 kuala lumpur", true},
		{"equals partial fails", models.RuleOpEquals, "grab", false},
		{"starts_with match", models.RuleOpStartsWith, "grab", true},
		{"starts_with no match", models.RuleOpStartsWith, "ride", false},
		{"ends_with match", models.RuleOpEndsWith, "lumpur", true},
		{"ends_with no match", models.RuleOpEndsWith, "grab", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule("r1", 0,
				[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, tt.operator, tt.value, nil, nil)},
				setCategoryAction("cat-exp", expenseCat("cat-exp")))
			res := Match([]models.TransactionRule{r}, expenseInput)
			got := res.CategoryID != nil
			if got != tt.want {
				t.Errorf("operator %s value %q: got match=%v, want %v", tt.operator, tt.value, got, tt.want)
			}
			if got && *res.CategoryID != "cat-exp" {
				t.Errorf("expected category cat-exp, got %s", *res.CategoryID)
			}
		})
	}
}

func TestMatch_AmountOperators(t *testing.T) {
	input := RuleInput{Description: "anything", Amount: 5000, Type: models.TransactionTypeExpense}

	tests := []struct {
		name string
		op   models.RuleOperator
		min  *int64
		max  *int64
		want bool
	}{
		{"gt below", models.RuleOpGt, i64Ptr(4000), nil, true},
		{"gt equal is not greater", models.RuleOpGt, i64Ptr(5000), nil, false},
		{"gt above", models.RuleOpGt, i64Ptr(6000), nil, false},
		{"lt above", models.RuleOpLt, nil, i64Ptr(6000), true},
		{"lt equal is not less", models.RuleOpLt, nil, i64Ptr(5000), false},
		{"between inclusive lower", models.RuleOpBetween, i64Ptr(5000), i64Ptr(9000), true},
		{"between inclusive upper", models.RuleOpBetween, i64Ptr(1000), i64Ptr(5000), true},
		{"between outside", models.RuleOpBetween, i64Ptr(1000), i64Ptr(4000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule("r1", 0,
				[]models.TransactionRuleCondition{cond(models.RuleFieldAmount, tt.op, "", tt.min, tt.max)},
				setCategoryAction("cat-exp", expenseCat("cat-exp")))
			res := Match([]models.TransactionRule{r}, input)
			if (res.CategoryID != nil) != tt.want {
				t.Errorf("amount op %s [%v,%v] amount=%d: got match=%v, want %v", tt.op, tt.min, tt.max, input.Amount, res.CategoryID != nil, tt.want)
			}
		})
	}
}

func TestMatch_AccountAndType(t *testing.T) {
	input := RuleInput{Description: "x", Amount: 100, AccountID: "acc-1", Type: models.TransactionTypeExpense}

	t.Run("account_id equals match", func(t *testing.T) {
		r := rule("r1", 0,
			[]models.TransactionRuleCondition{cond(models.RuleFieldAccountID, models.RuleOpEquals, "acc-1", nil, nil)},
			setCategoryAction("cat-exp", expenseCat("cat-exp")))
		if Match([]models.TransactionRule{r}, input).CategoryID == nil {
			t.Error("expected match on account_id")
		}
	})
	t.Run("account_id equals no match", func(t *testing.T) {
		r := rule("r1", 0,
			[]models.TransactionRuleCondition{cond(models.RuleFieldAccountID, models.RuleOpEquals, "acc-2", nil, nil)},
			setCategoryAction("cat-exp", expenseCat("cat-exp")))
		if Match([]models.TransactionRule{r}, input).CategoryID != nil {
			t.Error("expected no match on account_id")
		}
	})
	t.Run("type equals match", func(t *testing.T) {
		r := rule("r1", 0,
			[]models.TransactionRuleCondition{cond(models.RuleFieldType, models.RuleOpEquals, "expense", nil, nil)},
			setCategoryAction("cat-exp", expenseCat("cat-exp")))
		if Match([]models.TransactionRule{r}, input).CategoryID == nil {
			t.Error("expected match on type")
		}
	})
}

func TestMatch_AndSemantics(t *testing.T) {
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}
	conds := []models.TransactionRuleCondition{
		cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil),
		cond(models.RuleFieldAmount, models.RuleOpLt, "", nil, i64Ptr(5000)),
	}
	t.Run("all conditions pass", func(t *testing.T) {
		r := rule("r1", 0, conds, setCategoryAction("cat-exp", expenseCat("cat-exp")))
		if Match([]models.TransactionRule{r}, input).CategoryID == nil {
			t.Error("expected match when all AND conditions pass")
		}
	})
	t.Run("one condition fails => no match", func(t *testing.T) {
		failing := []models.TransactionRuleCondition{
			cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil),
			cond(models.RuleFieldAmount, models.RuleOpGt, "", i64Ptr(5000), nil), // 3000 !> 5000
		}
		r := rule("r1", 0, failing, setCategoryAction("cat-exp", expenseCat("cat-exp")))
		if Match([]models.TransactionRule{r}, input).CategoryID != nil {
			t.Error("expected no match when one AND condition fails")
		}
	})
}

func TestMatch_FirstMatchWinsByOrder(t *testing.T) {
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}
	// Both match; the slice is assumed pre-sorted priority ASC, so the first wins.
	r1 := rule("r1", 0,
		[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
		setCategoryAction("cat-transport", expenseCat("cat-transport")))
	r2 := rule("r2", 1,
		[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "ride", nil, nil)},
		setCategoryAction("cat-other", expenseCat("cat-other")))

	res := Match([]models.TransactionRule{r1, r2}, input)
	if res.CategoryID == nil || *res.CategoryID != "cat-transport" {
		t.Errorf("expected first rule to win (cat-transport), got %v", res.CategoryID)
	}
	if res.MatchedRuleID == nil || *res.MatchedRuleID != "r1" {
		t.Errorf("expected MatchedRuleID r1, got %v", res.MatchedRuleID)
	}
}

func TestMatch_TypeSafetySkip(t *testing.T) {
	// Rule matches by description but targets an INCOME category; the input is an
	// expense. The type-mismatched rule must be skipped so it never mis-types.
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}

	t.Run("type mismatch skipped", func(t *testing.T) {
		r := rule("r1", 0,
			[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
			setCategoryAction("cat-income", incomeCat("cat-income")))
		if Match([]models.TransactionRule{r}, input).CategoryID != nil {
			t.Error("expected type-mismatched rule to be skipped")
		}
	})

	t.Run("skips mismatch, later matching rule wins", func(t *testing.T) {
		bad := rule("r1", 0,
			[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
			setCategoryAction("cat-income", incomeCat("cat-income")))
		good := rule("r2", 1,
			[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
			setCategoryAction("cat-exp", expenseCat("cat-exp")))
		res := Match([]models.TransactionRule{bad, good}, input)
		if res.CategoryID == nil || *res.CategoryID != "cat-exp" {
			t.Errorf("expected fallthrough to cat-exp, got %v", res.CategoryID)
		}
	})
}

func TestMatch_SoftDeletedTargetSkipped(t *testing.T) {
	// A nil preloaded Category means the target was soft-deleted; skip the rule.
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}
	r := rule("r1", 0,
		[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
		setCategoryAction("cat-gone", nil))
	if Match([]models.TransactionRule{r}, input).CategoryID != nil {
		t.Error("expected rule with soft-deleted target category to be skipped")
	}
}

func TestMatch_InactiveRuleSkipped(t *testing.T) {
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}
	r := rule("r1", 0,
		[]models.TransactionRuleCondition{cond(models.RuleFieldDescription, models.RuleOpContains, "grab", nil, nil)},
		setCategoryAction("cat-exp", expenseCat("cat-exp")))
	r.IsActive = false
	if Match([]models.TransactionRule{r}, input).CategoryID != nil {
		t.Error("expected inactive rule to be skipped")
	}
}

func TestMatch_NoRules(t *testing.T) {
	input := RuleInput{Description: "GRAB RIDE", Amount: 3000, Type: models.TransactionTypeExpense}
	if Match(nil, input).CategoryID != nil {
		t.Error("expected no match with no rules")
	}
}

func TestMatch_EmptyConditionsNeverMatches(t *testing.T) {
	// A rule with zero conditions must not match everything (guards against a
	// vacuous-truth footgun). Validation forbids it, but the matcher defends too.
	input := RuleInput{Description: "x", Amount: 1, Type: models.TransactionTypeExpense}
	r := rule("r1", 0, nil, setCategoryAction("cat-exp", expenseCat("cat-exp")))
	if Match([]models.TransactionRule{r}, input).CategoryID != nil {
		t.Error("expected a rule with no conditions to never match")
	}
}
