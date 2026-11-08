package models

import (
	"time"

	"kuberan/internal/fx"
)

// Investment represents a holding of a specific investment asset.
type Investment struct {
	Base
	AccountID        string  `gorm:"type:uuid;not null" json:"account_id"`
	SecurityID       string  `gorm:"type:uuid;not null" json:"security_id"`
	Quantity         float64 `gorm:"not null" json:"quantity"`
	CostBasis        int64   `gorm:"type:bigint;not null" json:"cost_basis"`                   // security currency
	RealizedGainLoss int64   `gorm:"type:bigint;not null;default:0" json:"realized_gain_loss"` // security currency
	// CostBasisBase and RealizedGainLossBase are the same figures in the user's
	// base currency, frozen at each trade's FX rate (plan 024, phase 3a).
	CostBasisBase        int64  `gorm:"type:bigint;not null;default:0" json:"cost_basis_base"`
	RealizedGainLossBase int64  `gorm:"type:bigint;not null;default:0" json:"realized_gain_loss_base"`
	CurrentPrice         int64  `gorm:"-" json:"current_price"`  // Populated at query time: latest price in the user's base currency
	TotalInvested        int64  `gorm:"-" json:"total_invested"` // Populated at query time: sum of buy transaction amounts
	WalletAddress        string `json:"wallet_address,omitempty"`

	// Response-only price fields (plan 024, phase 2), populated at query time.
	CurrentPriceNative int64  `gorm:"-" json:"current_price_native"` // latest price in the security's own currency
	PriceCurrency      string `gorm:"-" json:"price_currency"`       // currency of CurrentPriceNative
	FXRate             string `gorm:"-" json:"fx_rate"`              // price currency -> base, decimal string; "" when already base
	FXStale            bool   `gorm:"-" json:"fx_stale"`             // the FX quote used was stale
	FXMissing          bool   `gorm:"-" json:"fx_missing"`           // no FX rate; CurrentPrice is 0
	Currency           string `gorm:"-" json:"currency"`             // the security's currency (same as price_currency)

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

	// Base-currency figures (plan 024, phase 3a). The amounts above are in the
	// security's currency. FXRateToBase is NULL on legacy rows.
	FXRateToBase         *fx.Rate `gorm:"type:numeric(20,10)" json:"fx_rate_to_base"`
	BaseAmount           *int64   `gorm:"type:bigint" json:"base_amount"`
	RealizedGainLossBase int64    `gorm:"type:bigint;not null;default:0" json:"realized_gain_loss_base"`
	Currency             string   `gorm:"-" json:"currency"` // response only: the security's currency

	// For splits
	SplitRatio float64 `json:"split_ratio,omitempty"`

	// For dividends
	DividendType string `json:"dividend_type,omitempty"` // Cash, Stock, Special

	// Relationships
	Investment Investment `gorm:"foreignKey:InvestmentID" json:"investment"`
}
