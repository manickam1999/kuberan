// Package validator provides custom validation functions for Gin's binding engine.
package validator

import (
	"regexp"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var hexColorRegex = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// validCurrencies contains ISO 4217 currency codes.
var validCurrencies = map[string]bool{
	"AED": true, "AFN": true, "ALL": true, "AMD": true, "ANG": true,
	"AOA": true, "ARS": true, "AUD": true, "AWG": true, "AZN": true,
	"BAM": true, "BBD": true, "BDT": true, "BGN": true, "BHD": true,
	"BIF": true, "BMD": true, "BND": true, "BOB": true, "BRL": true,
	"BSD": true, "BTN": true, "BWP": true, "BYN": true, "BZD": true,
	"CAD": true, "CDF": true, "CHF": true, "CLP": true, "CNY": true,
	"COP": true, "CRC": true, "CUP": true, "CVE": true, "CZK": true,
	"DJF": true, "DKK": true, "DOP": true, "DZD": true, "EGP": true,
	"ERN": true, "ETB": true, "EUR": true, "FJD": true, "FKP": true,
	"GBP": true, "GEL": true, "GHS": true, "GIP": true, "GMD": true,
	"GNF": true, "GTQ": true, "GYD": true, "HKD": true, "HNL": true,
	"HRK": true, "HTG": true, "HUF": true, "IDR": true, "ILS": true,
	"INR": true, "IQD": true, "IRR": true, "ISK": true, "JMD": true,
	"JOD": true, "JPY": true, "KES": true, "KGS": true, "KHR": true,
	"KMF": true, "KPW": true, "KRW": true, "KWD": true, "KYD": true,
	"KZT": true, "LAK": true, "LBP": true, "LKR": true, "LRD": true,
	"LSL": true, "LYD": true, "MAD": true, "MDL": true, "MGA": true,
	"MKD": true, "MMK": true, "MNT": true, "MOP": true, "MRU": true,
	"MUR": true, "MVR": true, "MWK": true, "MXN": true, "MYR": true,
	"MZN": true, "NAD": true, "NGN": true, "NIO": true, "NOK": true,
	"NPR": true, "NZD": true, "OMR": true, "PAB": true, "PEN": true,
	"PGK": true, "PHP": true, "PKR": true, "PLN": true, "PYG": true,
	"QAR": true, "RON": true, "RSD": true, "RUB": true, "RWF": true,
	"SAR": true, "SBD": true, "SCR": true, "SDG": true, "SEK": true,
	"SGD": true, "SHP": true, "SLE": true, "SOS": true, "SRD": true,
	"SSP": true, "STN": true, "SVC": true, "SYP": true, "SZL": true,
	"THB": true, "TJS": true, "TMT": true, "TND": true, "TOP": true,
	"TRY": true, "TTD": true, "TWD": true, "TZS": true, "UAH": true,
	"UGX": true, "USD": true, "UYU": true, "UZS": true, "VES": true,
	"VND": true, "VUV": true, "WST": true, "XAF": true, "XCD": true,
	"XOF": true, "XPF": true, "YER": true, "ZAR": true, "ZMW": true,
	"ZWL": true,
}

// Register registers all custom validators with the Gin binding engine.
func Register() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("iso4217", validateISO4217)
		_ = v.RegisterValidation("hex_color", validateHexColor)
		_ = v.RegisterValidation("transaction_type", validateTransactionType)
		_ = v.RegisterValidation("category_type", validateCategoryType)
		_ = v.RegisterValidation("account_type", validateAccountType)
		_ = v.RegisterValidation("budget_period", validateBudgetPeriod)
		_ = v.RegisterValidation("asset_type", validateAssetType)
	}
}

func validateISO4217(fl validator.FieldLevel) bool {
	return validCurrencies[fl.Field().String()]
}

func validateHexColor(fl validator.FieldLevel) bool {
	return hexColorRegex.MatchString(fl.Field().String())
}

func validateTransactionType(fl validator.FieldLevel) bool {
	switch fl.Field().String() {
	case "income", "expense", "transfer", "investment":
		return true
	}
	return false
}

func validateCategoryType(fl validator.FieldLevel) bool {
	switch fl.Field().String() {
	case "income", "expense":
		return true
	}
	return false
}

func validateAccountType(fl validator.FieldLevel) bool {
	switch fl.Field().String() {
	case "cash", "investment", "debt", "credit_card":
		return true
	}
	return false
}

func validateBudgetPeriod(fl validator.FieldLevel) bool {
	switch fl.Field().String() {
	case "monthly", "yearly":
		return true
	}
	return false
}

func validateAssetType(fl validator.FieldLevel) bool {
	switch fl.Field().String() {
	case "stock", "etf", "bond", "crypto", "reit", "commodity":
		return true
	}
	return false
}
