package models

// RuleField identifies which transaction attribute a condition matches against.
type RuleField string

const (
	RuleFieldDescription RuleField = "description"
	RuleFieldAmount      RuleField = "amount"
	RuleFieldAccountID   RuleField = "account_id"
	RuleFieldType        RuleField = "type"
)

// RuleOperator identifies how a condition compares its value.
type RuleOperator string

const (
	// Text operators (field: description). Comparison is case-insensitive.
	RuleOpContains    RuleOperator = "contains"
	RuleOpNotContains RuleOperator = "not_contains"
	RuleOpEquals      RuleOperator = "equals"
	RuleOpStartsWith  RuleOperator = "starts_with"
	RuleOpEndsWith    RuleOperator = "ends_with"
	// Numeric operators (field: amount, in cents).
	RuleOpGt      RuleOperator = "gt"
	RuleOpLt      RuleOperator = "lt"
	RuleOpBetween RuleOperator = "between"
)

// RuleActionType identifies what a matching rule does. v1 supports only
// set_category; add_tag/rename/hide are reserved for future additive types.
type RuleActionType string

const (
	RuleActionSetCategory RuleActionType = "set_category"
)

// TransactionRule is an if-this-then-that statement for auto-categorization.
// Conditions are AND-ed within a rule; separate rules act as OR. Rules are
// evaluated in priority order (lower first), and the first rule whose conditions
// all match and whose action is valid assigns the transaction's category.
type TransactionRule struct {
	Base
	UserID   string `gorm:"type:uuid;not null" json:"user_id"`
	Name     string `gorm:"not null" json:"name"`
	Priority int    `gorm:"not null;default:0" json:"priority"` // lower = evaluated first
	IsActive bool   `gorm:"not null;default:true" json:"is_active"`

	// Conditions and Actions are wholly-owned children, replaced on update.
	Conditions []TransactionRuleCondition `gorm:"foreignKey:RuleID" json:"conditions"`
	Actions    []TransactionRuleAction    `gorm:"foreignKey:RuleID" json:"actions"`
}

// TableName pins the table name (GORM would otherwise pluralize to
// "transaction_rules", which is what we want, but pin it for clarity).
func (TransactionRule) TableName() string { return "transaction_rules" }

// TransactionRuleCondition is a single AND-ed matching clause of a rule.
type TransactionRuleCondition struct {
	Base
	RuleID    string       `gorm:"type:uuid;not null;index" json:"rule_id"`
	Field     RuleField    `gorm:"not null" json:"field"`
	Operator  RuleOperator `gorm:"not null" json:"operator"`
	ValueText string       `json:"value_text"`           // text / account uuid / type value
	AmountMin *int64       `json:"amount_min,omitempty"` // cents (gt, between)
	AmountMax *int64       `json:"amount_max,omitempty"` // cents (lt, between)
}

// TableName pins the conditions table name.
func (TransactionRuleCondition) TableName() string { return "transaction_rule_conditions" }

// TransactionRuleAction is a single action a matching rule performs. v1: set_category.
type TransactionRuleAction struct {
	Base
	RuleID     string         `gorm:"type:uuid;not null;index" json:"rule_id"`
	ActionType RuleActionType `gorm:"not null" json:"action_type"`
	CategoryID *string        `gorm:"type:uuid" json:"category_id,omitempty"`
	ValueText  string         `json:"value_text,omitempty"` // reserved for future actions

	// Category is preloaded on reads so the matcher can check the target
	// category's type without a DB call. Nil when the target is soft-deleted.
	Category *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}

// TableName pins the actions table name.
func (TransactionRuleAction) TableName() string { return "transaction_rule_actions" }
