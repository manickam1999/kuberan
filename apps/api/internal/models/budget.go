package models

// BudgetPeriod represents the period type for a budget
type BudgetPeriod string

const (
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodYearly  BudgetPeriod = "yearly"
)

// Budget represents a budget plan for a category
type Budget struct {
	Base
	UserID     string       `gorm:"type:uuid;not null" json:"user_id"`
	CategoryID string       `gorm:"type:uuid;not null" json:"category_id"`
	Name       string       `gorm:"not null" json:"name"`
	Amount     int64        `gorm:"type:bigint;not null" json:"amount"`
	Period     BudgetPeriod `gorm:"not null" json:"period"`
	IsActive   bool         `gorm:"default:true" json:"is_active"`

	// Relationships
	Category Category `gorm:"foreignKey:CategoryID" json:"category"`
}
