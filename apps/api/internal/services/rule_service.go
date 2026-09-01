package services

import (
	"errors"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// ruleService handles transaction-rule business logic (plan 018).
type ruleService struct {
	db              *gorm.DB
	categoryService CategoryServicer
}

// NewRuleService creates a new RuleServicer. It depends only on the DB and the
// category service (to validate action targets); it never depends on the
// transaction service, keeping the dependency direction one-way.
func NewRuleService(db *gorm.DB, categoryService CategoryServicer) RuleServicer {
	return &ruleService{db: db, categoryService: categoryService}
}

// rulePreloads applies the standard preloads so the matcher sees conditions,
// actions, and each action's target category type without extra queries.
func rulePreloads(q *gorm.DB) *gorm.DB {
	return q.Preload("Conditions").Preload("Actions").Preload("Actions.Category")
}

// CreateRule validates and persists a new rule with its conditions and actions.
func (s *ruleService) CreateRule(userID string, in CreateRuleInput) (*models.TransactionRule, error) {
	if err := s.validateChildren(userID, in.Conditions, in.Actions); err != nil {
		return nil, err
	}

	rule := &models.TransactionRule{
		UserID:   userID,
		Name:     in.Name,
		Priority: in.Priority,
		IsActive: true,
	}
	if in.IsActive != nil {
		rule.IsActive = *in.IsActive
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return s.insertChildren(tx, rule.ID, in.Conditions, in.Actions)
	})
	if err != nil {
		return nil, err
	}

	return s.GetRule(userID, rule.ID)
}

// GetRule returns a rule (with children preloaded) if it belongs to the user.
func (s *ruleService) GetRule(userID, ruleID string) (*models.TransactionRule, error) {
	var rule models.TransactionRule
	if err := rulePreloads(s.db).Where("id = ? AND user_id = ?", ruleID, userID).First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRuleNotFound
		}
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return &rule, nil
}

// ListRules returns all of a user's rules in evaluation order
// (priority ASC, created_at ASC) with children preloaded.
func (s *ruleService) ListRules(userID string) ([]models.TransactionRule, error) {
	var rules []models.TransactionRule
	if err := rulePreloads(s.db).
		Where("user_id = ?", userID).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return rules, nil
}

// UpdateRule updates a rule's fields and, when provided, replaces its conditions
// and/or actions wholesale (hard delete + reinsert within one transaction).
func (s *ruleService) UpdateRule(userID, ruleID string, in UpdateRuleInput) (*models.TransactionRule, error) {
	rule, err := s.GetRule(userID, ruleID)
	if err != nil {
		return nil, err
	}

	// Determine the effective children for validation: incoming when provided,
	// otherwise the rule's current children mapped back to inputs.
	conditions := in.Conditions
	actions := in.Actions
	if conditions != nil || actions != nil {
		effConditions := conditions
		if effConditions == nil {
			effConditions = conditionsToInputs(rule.Conditions)
		}
		effActions := actions
		if effActions == nil {
			effActions = actionsToInputs(rule.Actions)
		}
		if err := s.validateChildren(userID, effConditions, effActions); err != nil {
			return nil, err
		}
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{}
		if in.Name != nil {
			updates["name"] = *in.Name
		}
		if in.Priority != nil {
			updates["priority"] = *in.Priority
		}
		if in.IsActive != nil {
			updates["is_active"] = *in.IsActive
		}
		if len(updates) > 0 {
			if err := tx.Model(rule).Updates(updates).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}

		if in.Conditions != nil {
			if err := tx.Unscoped().Where("rule_id = ?", ruleID).Delete(&models.TransactionRuleCondition{}).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			if err := s.insertConditions(tx, ruleID, in.Conditions); err != nil {
				return err
			}
		}
		if in.Actions != nil {
			if err := tx.Unscoped().Where("rule_id = ?", ruleID).Delete(&models.TransactionRuleAction{}).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
			if err := s.insertActions(tx, ruleID, in.Actions); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetRule(userID, ruleID)
}

// DeleteRule soft-deletes a rule and hard-deletes its owned children.
func (s *ruleService) DeleteRule(userID, ruleID string) error {
	rule, err := s.GetRule(userID, ruleID)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("rule_id = ?", ruleID).Delete(&models.TransactionRuleCondition{}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if err := tx.Unscoped().Where("rule_id = ?", ruleID).Delete(&models.TransactionRuleAction{}).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		if err := tx.Delete(rule).Error; err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		return nil
	})
}

// ReorderRules rewrites priorities to match the given order (index = priority)
// for the user's rules, in one transaction. Unknown or foreign IDs are ignored.
func (s *ruleService) ReorderRules(userID string, ruleIDs []string) ([]models.TransactionRule, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ruleIDs {
			if err := tx.Model(&models.TransactionRule{}).
				Where("id = ? AND user_id = ?", id, userID).
				Update("priority", i).Error; err != nil {
				return apperrors.Wrap(apperrors.ErrInternalServer, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListRules(userID)
}

// ResolveForUser loads the user's active rules (ordered for evaluation) and
// returns the category the input transaction should receive, if any.
func (s *ruleService) ResolveForUser(userID string, in RuleInput) (RuleResult, error) {
	var rules []models.TransactionRule
	if err := rulePreloads(s.db).
		Where("user_id = ? AND is_active = ?", userID, true).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return RuleResult{}, apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return Match(rules, in), nil
}

// insertChildren persists conditions and actions for a rule.
func (s *ruleService) insertChildren(tx *gorm.DB, ruleID string, conditions []RuleConditionInput, actions []RuleActionInput) error {
	if err := s.insertConditions(tx, ruleID, conditions); err != nil {
		return err
	}
	return s.insertActions(tx, ruleID, actions)
}

func (s *ruleService) insertConditions(tx *gorm.DB, ruleID string, conditions []RuleConditionInput) error {
	if len(conditions) == 0 {
		return nil
	}
	rows := make([]models.TransactionRuleCondition, len(conditions))
	for i, c := range conditions {
		rows[i] = models.TransactionRuleCondition{
			RuleID:    ruleID,
			Field:     c.Field,
			Operator:  c.Operator,
			ValueText: c.ValueText,
			AmountMin: c.AmountMin,
			AmountMax: c.AmountMax,
		}
	}
	if err := tx.Create(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

func (s *ruleService) insertActions(tx *gorm.DB, ruleID string, actions []RuleActionInput) error {
	if len(actions) == 0 {
		return nil
	}
	rows := make([]models.TransactionRuleAction, len(actions))
	for i, a := range actions {
		rows[i] = models.TransactionRuleAction{
			RuleID:     ruleID,
			ActionType: a.ActionType,
			CategoryID: a.CategoryID,
			ValueText:  a.ValueText,
		}
	}
	if err := tx.Create(&rows).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// validateChildren validates a rule's conditions and actions at write time.
func (s *ruleService) validateChildren(userID string, conditions []RuleConditionInput, actions []RuleActionInput) error {
	if len(conditions) == 0 {
		return apperrors.WithMessage(apperrors.ErrRuleInvalid, "a rule must have at least one condition")
	}
	if len(actions) == 0 {
		return apperrors.WithMessage(apperrors.ErrRuleInvalid, "a rule must have at least one action")
	}

	// The transaction type a rule targets, if constrained by a `type` condition.
	// Used to cross-check the action's target category type.
	var typeCondition *models.TransactionType
	for i := range conditions {
		if err := s.validateCondition(userID, &conditions[i]); err != nil {
			return err
		}
		if conditions[i].Field == models.RuleFieldType {
			t := models.TransactionType(conditions[i].ValueText)
			typeCondition = &t
		}
	}

	for i := range actions {
		if err := s.validateAction(userID, &actions[i], typeCondition); err != nil {
			return err
		}
	}
	return nil
}

// validateCondition checks operator/field compatibility and value presence.
func (s *ruleService) validateCondition(userID string, c *RuleConditionInput) error {
	switch c.Field {
	case models.RuleFieldDescription:
		if !isTextOperator(c.Operator) {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "operator not valid for description")
		}
		if c.ValueText == "" {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "description condition requires a value")
		}
	case models.RuleFieldAmount:
		return validateAmountCondition(c)
	case models.RuleFieldAccountID:
		if c.Operator != models.RuleOpEquals {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "account_id condition requires the equals operator")
		}
		if err := s.assertAccountOwned(userID, c.ValueText); err != nil {
			return err
		}
	case models.RuleFieldType:
		if c.Operator != models.RuleOpEquals {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "type condition requires the equals operator")
		}
		if c.ValueText != string(models.TransactionTypeIncome) && c.ValueText != string(models.TransactionTypeExpense) {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "type condition value must be 'income' or 'expense'")
		}
	default:
		return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "unknown condition field")
	}
	return nil
}

func validateAmountCondition(c *RuleConditionInput) error {
	switch c.Operator {
	case models.RuleOpGt:
		if c.AmountMin == nil {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "gt requires amount_min")
		}
	case models.RuleOpLt:
		if c.AmountMax == nil {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "lt requires amount_max")
		}
	case models.RuleOpBetween:
		if c.AmountMin == nil || c.AmountMax == nil {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "between requires amount_min and amount_max")
		}
		if *c.AmountMin > *c.AmountMax {
			return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "amount_min must be <= amount_max")
		}
	default:
		return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "operator not valid for amount")
	}
	return nil
}

// validateAction checks the action type and, for set_category, that the target
// category is owned by the user and (if the rule constrains transaction type)
// that the category's type matches.
func (s *ruleService) validateAction(userID string, a *RuleActionInput, typeCondition *models.TransactionType) error {
	if a.ActionType != models.RuleActionSetCategory {
		return apperrors.WithMessage(apperrors.ErrRuleActionInvalid, "unsupported action type")
	}
	if a.CategoryID == nil || *a.CategoryID == "" {
		return apperrors.WithMessage(apperrors.ErrRuleActionInvalid, "set_category requires a category_id")
	}

	category, err := s.categoryService.GetCategoryByID(userID, *a.CategoryID)
	if err != nil {
		return err
	}

	if typeCondition != nil {
		if !categoryTypeMatchesTxType(category.Type, *typeCondition) {
			return apperrors.ErrRuleCategoryTypeMismatch
		}
	}
	return nil
}

// assertAccountOwned confirms an account exists and belongs to the user.
func (s *ruleService) assertAccountOwned(userID, accountID string) error {
	if accountID == "" {
		return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "account_id condition requires a value")
	}
	var count int64
	if err := s.db.Model(&models.Account{}).
		Where("id = ? AND user_id = ?", accountID, userID).
		Count(&count).Error; err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	if count == 0 {
		return apperrors.WithMessage(apperrors.ErrRuleConditionInvalid, "account not found")
	}
	return nil
}

func isTextOperator(op models.RuleOperator) bool {
	switch op {
	case models.RuleOpContains, models.RuleOpNotContains, models.RuleOpEquals,
		models.RuleOpStartsWith, models.RuleOpEndsWith:
		return true
	default:
		return false
	}
}

// conditionsToInputs / actionsToInputs map persisted children back to inputs so
// an update that touches only one child set can revalidate against the other.
func conditionsToInputs(conditions []models.TransactionRuleCondition) []RuleConditionInput {
	out := make([]RuleConditionInput, len(conditions))
	for i := range conditions {
		c := &conditions[i]
		out[i] = RuleConditionInput{
			Field:     c.Field,
			Operator:  c.Operator,
			ValueText: c.ValueText,
			AmountMin: c.AmountMin,
			AmountMax: c.AmountMax,
		}
	}
	return out
}

func actionsToInputs(actions []models.TransactionRuleAction) []RuleActionInput {
	out := make([]RuleActionInput, len(actions))
	for i := range actions {
		a := &actions[i]
		out[i] = RuleActionInput{
			ActionType: a.ActionType,
			CategoryID: a.CategoryID,
			ValueText:  a.ValueText,
		}
	}
	return out
}
