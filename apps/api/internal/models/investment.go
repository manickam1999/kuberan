package models

import "time"

// Investment represents a holding of a specific investment asset.
type Investment struct {
	Base
	AccountID        string  `gorm:"type:uuid;not null" json:"account_id"`
	SecurityID       string  `gorm:"type:uuid;not null" json:"security_id"`
	Quantity         float64 `gorm:"not null" json:"quantity"`
	CostBasis        int64   `gorm:"type:bigint;not null" json:"cost_basis"`
	RealizedGainLoss int64   `gorm:"type:bigint;not null;default:0" json:"realized_gain_loss"`
	CurrentPrice     int64   `gorm:"-" json:"current_price"`  // Populated at query time from security_prices
	TotalInvested    int64   `gorm:"-" json:"total_invested"` // Populated at query time: sum of buy transaction amounts
	WalletAddress    string  `json:"wallet_address,omitempty"`

	// Relationships
	Security     Security                `gorm:"foreignKey:SecurityID" json:"security"`
	Account      Account                 `gorm:"foreignKey:AccountID" json:"account"`
	Transactions []InvestmentTransaction `gorm:"foreignKey:InvestmentID" json:"transactions,omitempty"`
}

// InvestmentTransactionType represents the type of investment transaction.
type InvestmentTransactionType string

const (
	InvestmentTransactionBuy      InvestmentTransactionType = "buy"
	InvestmentTransactionSell     InvestmentTransactionType = "sell"
	InvestmentTransactionDividend InvestmentTransactionType = "dividend"
	InvestmentTransactionSplit    InvestmentTransactionType = "split"
	InvestmentTransactionTransfer InvestmentTransactionType = "transfer"
)

// InvestmentTransaction represents a transaction for an investment.
type InvestmentTransaction struct {
	Base
	InvestmentID     string                    `gorm:"type:uuid;not null" json:"investment_id"`
	Type             InvestmentTransactionType `gorm:"not null" json:"type"`
	Date             time.Time                 `gorm:"not null" json:"date"`
	Quantity         float64                   `gorm:"not null" json:"quantity"`
	PricePerUnit     int64                     `gorm:"type:bigint;not null" json:"price_per_unit"`
	TotalAmount      int64                     `gorm:"type:bigint;not null" json:"total_amount"`
	Fee              int64                     `gorm:"type:bigint" json:"fee"`
	Notes            string                    `json:"notes"`
	RealizedGainLoss int64                     `gorm:"type:bigint;not null;default:0" json:"realized_gain_loss"`

	// For splits
	SplitRatio float64 `json:"split_ratio,omitempty"`

	// For dividends
	DividendType string `json:"dividend_type,omitempty"` // Cash, Stock, Special

	// Relationships
	Investment Investment `gorm:"foreignKey:InvestmentID" json:"investment"`
}
