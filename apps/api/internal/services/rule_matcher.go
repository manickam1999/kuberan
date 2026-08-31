package services

import (
	"strings"

	"kuberan/internal/models"
)

// RuleInput is the minimal, DB-free view of a transaction the matcher needs.
type RuleInput struct {
	Description string
	Amount      int64 // cents
	AccountID   string
	Type        models.TransactionType
}

// RuleResult is the outcome of evaluating rules against an input. It is a result
// object (not a bare *categoryID) so future accumulating actions (e.g. tags) can
// be added without changing the matcher's contract.
type RuleResult struct {
	CategoryID    *string // first matching terminal set_category (nil if none)
	MatchedRuleID *string // the rule that assigned the category
}

// Match evaluates rules against an input and returns the resolved category.
//
// It is a PURE function: no database access, no clock, no user lookup. Callers
// must pass rules that are already loaded with Conditions + Actions +
// Action.Category preloaded, and pre-sorted by (priority ASC, created_at ASC).
// The first active rule whose conditions all match (AND) and whose set_category
// action targets a valid category (not soft-deleted, type matching the input)
// assigns the category, and evaluation stops (terminal first-match-wins).
func Match(rules []models.TransactionRule, in RuleInput) RuleResult {
	for i := range rules {
		r := &rules[i]
		if !r.IsActive {
			continue
		}
		if !ConditionsMatch(r.Conditions, in) {
			continue
		}
		if cat := setCategoryTarget(r, in); cat != nil {
			ruleID := r.ID
			return RuleResult{CategoryID: cat, MatchedRuleID: &ruleID}
		}
	}
	return RuleResult{}
}

// ConditionsMatch reports whether every condition passes (AND). An empty
// condition set never matches, guarding against vacuous truth. Exported for
// match preview over unsaved conditions.
func ConditionsMatch(conditions []models.TransactionRuleCondition, in RuleInput) bool {
	if len(conditions) == 0 {
		return false
	}
	for i := range conditions {
		if !conditionMatches(&conditions[i], in) {
			return false
		}
	}
	return true
}

// setCategoryTarget returns the category ID a valid set_category action would
// assign for this input, or nil if the rule has no honorable set_category action
// (target soft-deleted, or its type does not match the transaction type).
func setCategoryTarget(r *models.TransactionRule, in RuleInput) *string {
	for i := range r.Actions {
		a := &r.Actions[i]
		if a.ActionType != models.RuleActionSetCategory || a.CategoryID == nil {
			continue
		}
		// Category is nil when the target was soft-deleted (default GORM scope
		// excludes it from the preload). Skip such rules defensively.
		if a.Category == nil {
			continue
		}
		if !categoryTypeMatchesTxType(a.Category.Type, in.Type) {
			continue
		}
		return a.CategoryID
	}
	return nil
}

// categoryTypeMatchesTxType ensures we never attach an income category to an
// expense transaction (or vice versa), which would corrupt spending reports.
func categoryTypeMatchesTxType(catType models.CategoryType, txType models.TransactionType) bool {
	switch txType {
	case models.TransactionTypeExpense:
		return catType == models.CategoryTypeExpense
	case models.TransactionTypeIncome:
		return catType == models.CategoryTypeIncome
	default:
		return false
	}
}

// conditionMatches evaluates a single condition against the input.
func conditionMatches(c *models.TransactionRuleCondition, in RuleInput) bool {
	switch c.Field {
	case models.RuleFieldDescription:
		return textConditionMatches(c.Operator, in.Description, c.ValueText)
	case models.RuleFieldAmount:
		return amountConditionMatches(c.Operator, in.Amount, c.AmountMin, c.AmountMax)
	case models.RuleFieldAccountID:
		return c.Operator == models.RuleOpEquals && in.AccountID == c.ValueText
	case models.RuleFieldType:
		return c.Operator == models.RuleOpEquals && string(in.Type) == c.ValueText
	default:
		return false
	}
}

// textConditionMatches compares description text case-insensitively.
func textConditionMatches(op models.RuleOperator, subject, value string) bool {
	s := strings.ToLower(subject)
	v := strings.ToLower(value)
	switch op {
	case models.RuleOpContains:
		return strings.Contains(s, v)
	case models.RuleOpNotContains:
		return !strings.Contains(s, v)
	case models.RuleOpEquals:
		return s == v
	case models.RuleOpStartsWith:
		return strings.HasPrefix(s, v)
	case models.RuleOpEndsWith:
		return strings.HasSuffix(s, v)
	default:
		return false
	}
}

// amountConditionMatches compares the amount (cents) using the numeric operators.
// A missing bound (nil) means the condition cannot be satisfied.
func amountConditionMatches(op models.RuleOperator, amount int64, min, max *int64) bool {
	switch op {
	case models.RuleOpGt:
		return min != nil && amount > *min
	case models.RuleOpLt:
		return max != nil && amount < *max
	case models.RuleOpBetween:
		return min != nil && max != nil && amount >= *min && amount <= *max
	default:
		return false
	}
}
