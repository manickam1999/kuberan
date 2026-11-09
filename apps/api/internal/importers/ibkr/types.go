package ibkr

import (
	"fmt"
	"math/big"
	"time"
)

// ParseError describes a problem found while parsing one statement file.
type ParseError struct {
	// Line is the 1-based line number in the CSV (0 when not row specific).
	Line int
	// Section is the statement section the row belongs to.
	Section string
	// Msg describes the problem.
	Msg string
}

// Error implements error.
func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("ibkr: line %d (%s): %s", e.Line, e.Section, e.Msg)
	}
	return fmt.Sprintf("ibkr: %s", e.Msg)
}

// Statement is the parsed content of one Activity Statement CSV.
type Statement struct {
	// AccountID is the IBKR account id (Account Information / Account).
	AccountID string
	// BaseCurrency is the account base currency (Account Information / Base Currency).
	BaseCurrency string
	// PeriodStart and PeriodEnd are the inclusive statement period as civil
	// dates (UTC midnight).
	PeriodStart, PeriodEnd time.Time

	// Trades are the Trades/Order rows for Stocks.
	Trades []TradeRow
	// Dividends are the Dividends data rows (Total rows excluded).
	Dividends []CashRow
	// Withholdings are the Withholding Tax data rows.
	Withholdings []CashRow
	// Fees are the Fees / Other Fees data rows.
	Fees []CashRow
	// Deposits are the Deposits & Withdrawals data rows.
	Deposits []CashRow
	// Grants are the Grant Activity data rows.
	Grants []GrantRow
	// CorporateActions are the Corporate Actions data rows (splits only).
	CorporateActions []SplitRow

	// Positions are the Open Positions Summary rows keyed by symbol.
	Positions map[string]Position
	// EndingCashCents is the Cash Report "Ending Cash" (Base Currency Summary
	// total) in cents; HasEndingCash reports whether the row was present.
	EndingCashCents int64
	// HasEndingCash is true when the Cash Report had an Ending Cash row.
	HasEndingCash bool
	// Instruments are the Financial Instrument Information rows keyed by symbol.
	Instruments map[string]Instrument
	// IgnoredSections lists, in first-seen order, the sections that were
	// recognised and deliberately not used.
	IgnoredSections []string
}

// TradeRow is one Trades/Order row for a stock.
type TradeRow struct {
	// Line is the CSV line number.
	Line int
	// Symbol is the ticker.
	Symbol string
	// Currency is the trade currency.
	Currency string
	// Time is the trade time in UTC (the statement prints America/New_York).
	Time time.Time
	// Quantity is signed (> 0 buy, < 0 sell), rounded to 10 dp.
	Quantity float64
	// Price is the exact T. Price.
	Price *big.Rat
	// Proceeds is the exact Proceeds column (negative for buys).
	Proceeds *big.Rat
	// Fee is the exact Comm/Fee column (negative cost).
	Fee *big.Rat
}

// CashRow is a dated, signed amount row (dividend, withholding, fee, deposit).
type CashRow struct {
	// Line is the CSV line number.
	Line int
	// Symbol is the text before "(" in the description (empty for deposits).
	Symbol string
	// Currency is the row currency.
	Currency string
	// Date is the civil date (UTC midnight): Date or Settle Date.
	Date time.Time
	// Subtitle is the Fees sub-section (e.g. "Other Fees"), empty elsewhere.
	Subtitle string
	// Description is IBKR's description.
	Description string
	// Amount is the exact signed amount.
	Amount *big.Rat
}

// GrantRow is one Grant Activity row.
type GrantRow struct {
	// Line is the CSV line number.
	Line int
	// Symbol is the ticker.
	Symbol string
	// ReportDate is the civil Report Date (UTC midnight).
	ReportDate time.Time
	// Description is e.g. "Stock Award Grant for Cash Deposit".
	Description string
	// Quantity is signed, rounded to 10 dp.
	Quantity float64
	// Price is the exact Price column.
	Price *big.Rat
	// Value is the exact signed Value column.
	Value *big.Rat
}

// SplitRow is a Corporate Actions split row.
type SplitRow struct {
	// Line is the CSV line number.
	Line int
	// Symbol is the ticker.
	Symbol string
	// Currency is the row currency.
	Currency string
	// Time is the Date/Time column in UTC.
	Time time.Time
	// Description is IBKR's description.
	Description string
	// Numerator and Denominator are N and M of "Split N for M".
	Numerator, Denominator int
}

// Position is an Open Positions Summary checkpoint row.
type Position struct {
	// Line is the CSV line number.
	Line int
	// Currency is the position currency.
	Currency string
	// Quantity is rounded to 10 dp.
	Quantity float64
	// CostBasisCents is the native cost basis in cents.
	CostBasisCents int64
}

// Instrument is a Financial Instrument Information row.
type Instrument struct {
	// Symbol is the ticker.
	Symbol string
	// Description is the instrument name.
	Description string
	// ISIN is the "Security ID" column.
	ISIN string
	// ListingExchange is the "Listing Exch" column (e.g. "LSEETF").
	ListingExchange string
	// Type is the "Type" column (COMMON, ETF, ADR, ...).
	Type string
}
