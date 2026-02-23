package models

import "time"

// TelegramLink represents a link between a Telegram account and a Kuberan user
type TelegramLink struct {
	Base
	UserID             string     `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	TelegramUserID     int64      `gorm:"uniqueIndex;not null" json:"telegram_user_id"`
	TelegramUsername   string     `json:"telegram_username,omitempty"`
	TelegramFirstName  string     `json:"telegram_first_name,omitempty"`
	LinkCode           string     `gorm:"size:6" json:"-"`
	LinkCodeExpiresAt  *time.Time `json:"-"`
	DefaultCurrency    string     `gorm:"size:3;not null;default:'MYR'" json:"default_currency"`
	IsActive           bool       `gorm:"default:true" json:"is_active"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	MessageCount       int64      `gorm:"default:0" json:"message_count"`
	User               User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
