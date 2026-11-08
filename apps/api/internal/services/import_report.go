package services

// IBKRImportReport is the result of an IBKR import, identical for a dry run
// and an applied import (plan 024, phase 3b). It contains no file contents and
// no account holder name.
type IBKRImportReport struct {
	// Applied is true when the changes were written.
	Applied bool `json:"applied"`
	// Account is the IBKR account id found in the statements.
	Account string `json:"account"`
	// BaseCurrency is the statements' base currency.
	BaseCurrency string `json:"base_currency"`
	// Periods are the statement periods in chronological order.
	Periods []IBKRImportPeriod `json:"periods"`
	// IgnoredSections lists the statement sections that are deliberately not imported.
	IgnoredSections []string `json:"ignored_sections"`
	// Securities has one entry per symbol with holdings activity.
	Securities []IBKRImportSecurity `json:"securities"`
	// Cash describes the cash ledger part of the import.
	Cash IBKRImportCash `json:"cash"`
	// Errors block an apply.
	Errors []IBKRImportIssue `json:"errors"`
	// Warnings never block an apply.
	Warnings []string `json:"warnings"`
}

// IBKRImportPeriod is one statement's inclusive date range.
type IBKRImportPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// IBKRImportIssue is an error in the report.
type IBKRImportIssue struct {
	// Code is machine readable, e.g. unknown_security, currency_mismatch,
	// fx_rate_missing, insufficient_shares, opening_balance_missing.
	Code string `json:"code"`
	// Detail describes the problem for the operator.
	Detail string `json:"detail"`
}

// IBKRImportSecurity is the plan for one symbol.
type IBKRImportSecurity struct {
	Symbol     string `json:"symbol"`
	SecurityID string `json:"security_id"`
	// InvestmentID is null when the investment will be created on apply.
	InvestmentID         *string `json:"investment_id"`
	ExistingTransactions int64   `json:"existing_transactions"`
	NewTransactions      int     `json:"new_transactions"`
	// Final is the holding after replaying all events.
	Final       IBKRImportFinal        `json:"final"`
	Checkpoints []IBKRImportCheckpoint `json:"checkpoints"`
}

// IBKRImportFinal is the replayed holding state.
type IBKRImportFinal struct {
	Quantity             float64 `json:"quantity"`
	CostBasis            int64   `json:"cost_basis"`
	CostBasisBase        int64   `json:"cost_basis_base"`
	RealizedGainLoss     int64   `json:"realized_gain_loss"`
	RealizedGainLossBase int64   `json:"realized_gain_loss_base"`
}

// IBKRImportCheckpoint compares the replay with the broker's Open Positions
// summary at a statement end date.
type IBKRImportCheckpoint struct {
	Date          string  `json:"date"`
	IBKRQuantity  float64 `json:"ibkr_quantity"`
	Quantity      float64 `json:"quantity"`
	QuantityOK    bool    `json:"quantity_ok"`
	IBKRCostBasis int64   `json:"ibkr_cost_basis"`
	CostBasis     int64   `json:"cost_basis"`
	// CostBasisDiff is cost_basis - ibkr_cost_basis (FIFO vs average cost).
	CostBasisDiff int64 `json:"cost_basis_diff"`
}

// IBKRImportCash describes the cash ledger part of the import.
type IBKRImportCash struct {
	// OpeningBalance is the opening balance in cents, from the Ending Cash of
	// the statement that ends the day before cash_start_date.
	OpeningBalance int64 `json:"opening_balance"`
	// Legs counts trade cash legs; Dividends dividend legs; Fees standalone
	// fee and withholding expenses.
	Legs      int `json:"legs"`
	Dividends int `json:"dividends"`
	Fees      int `json:"fees"`
	// DepositsMatched are the legacy transfers re-pointed (or already pointed)
	// at the new cash account.
	DepositsMatched []IBKRImportDeposit `json:"deposits_matched"`
	// UnmatchedDeposits are deposits and withdrawals with no legacy transfer;
	// the owner records them by hand.
	UnmatchedDeposits []IBKRImportUnmatched `json:"unmatched_deposits"`
	// Checkpoints compare the ledger with Ending Cash at each statement end.
	Checkpoints []IBKRImportCashCheckpoint `json:"checkpoints"`
	// LegacyAccountBalanceAfter is the legacy account's balance after the
	// re-pointing; null without a legacy account.
	LegacyAccountBalanceAfter *int64 `json:"legacy_account_balance_after"`
}

// IBKRImportDeposit is a deposit matched to a transfer.
type IBKRImportDeposit struct {
	Date          string `json:"date"`
	Amount        int64  `json:"amount"`
	TransactionID string `json:"transaction_id"`
	// SourceAmount is the transfer's amount in the source account's currency.
	SourceAmount int64 `json:"source_amount"`
}

// IBKRImportUnmatched is a deposit or withdrawal with no matching transfer.
type IBKRImportUnmatched struct {
	Date   string `json:"date"`
	Kind   string `json:"kind"`
	Amount int64  `json:"amount"`
	Detail string `json:"detail"`
	// Candidates are the legacy transfers that were within 5 days but rejected
	// (implied rate too far from the market rate), to help manual pairing.
	Candidates []IBKRImportCandidate `json:"candidates"`
}

// IBKRImportCandidate is a rejected transfer candidate for a deposit.
type IBKRImportCandidate struct {
	TransactionID string `json:"transaction_id"`
	Date          string `json:"date"`
	// Amount is the transfer's amount in the source account's currency.
	Amount int64 `json:"amount"`
	// DeviationPct is how far the implied rate is from the market rate, in percent.
	DeviationPct float64 `json:"deviation_pct"`
}

// IBKRImportCashCheckpoint compares the ledger with the broker's Ending Cash.
type IBKRImportCashCheckpoint struct {
	Date   string `json:"date"`
	IBKR   int64  `json:"ibkr"`
	Ledger int64  `json:"ledger"`
	// Diff is ibkr - ledger.
	Diff int64 `json:"diff"`
}
