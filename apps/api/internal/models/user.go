package models

import "time"

// User represents the user model in the database
type User struct {
	Base
	Email               string        `gorm:"uniqueIndex;not null" json:"email"`
	Password            string        `gorm:"not null" json:"-"`
	FirstName           string        `json:"first_name"`
	LastName            string        `json:"last_name"`
	IsActive            bool          `gorm:"default:true" json:"is_active"`
	RefreshTokenHash    string        `gorm:"size:64" json:"-"`
	FailedLoginAttempts int           `gorm:"default:0" json:"-"`
	LockedUntil         *time.Time    `json:"-"`
	LastLoginAt         *time.Time    `json:"last_login_at,omitempty"`
	Accounts            []Account     `gorm:"foreignKey:UserID" json:"accounts,omitempty"`
	Budgets             []Budget      `gorm:"foreignKey:UserID" json:"budgets,omitempty"`
	Categories          []Category    `gorm:"foreignKey:UserID" json:"categories,omitempty"`
	Transactions        []Transaction `gorm:"foreignKey:UserID" json:"transactions,omitempty"`
}
