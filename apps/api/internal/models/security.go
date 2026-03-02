package models

import "time"

// AssetType represents the type of investment asset.
type AssetType string

const (
	AssetTypeStock     AssetType = "stock"
	AssetTypeETF       AssetType = "etf"
	AssetTypeBond      AssetType = "bond"
	AssetTypeCrypto    AssetType = "crypto"
	AssetTypeREIT      AssetType = "reit"
	AssetTypeCommodity AssetType = "commodity"
)

// Security represents a normalized financial instrument (stock, ETF, bond, etc.).
type Security struct {
	Base
	Symbol          string     `gorm:"not null;uniqueIndex:uq_securities_symbol_exchange" json:"symbol"`
	Name            string     `gorm:"not null" json:"name"`
	AssetType       AssetType  `gorm:"not null" json:"asset_type"`
	Currency        string     `gorm:"not null;default:'USD'" json:"currency"`
	Exchange        string     `gorm:"uniqueIndex:uq_securities_symbol_exchange" json:"exchange,omitempty"`
	ProviderSymbol  string     `gorm:"default:''" json:"provider_symbol,omitempty"`
	MaturityDate    *time.Time `json:"maturity_date,omitempty"`
	YieldToMaturity float64    `json:"yield_to_maturity,omitempty"`
	CouponRate      float64    `json:"coupon_rate,omitempty"`
	Network         string     `json:"network,omitempty"`
	PropertyType    string     `json:"property_type,omitempty"`
}
