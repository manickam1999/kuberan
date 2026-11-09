package services

import (
	"io"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// UserServicer defines the contract for user-related business logic.
type UserServicer interface {
	CreateUser(email, password, firstName, lastName string, opts ...CreateUserOption) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)
	VerifyPassword(user *models.User, password string) bool
	AttemptLogin(email, password string) (*models.User, error)
	CreateRefreshSession(userID, tokenHash, userAgent, ipAddress string, expiresAt time.Time) error
	RotateRefreshSession(userID, oldHash, newHash string, expiresAt time.Time) error
	RevokeRefreshSession(userID, tokenHash string) error
	FindRefreshSessionID(userID, tokenHash string) (string, error)
	ListRefreshSessions(userID string) ([]models.RefreshSession, error)
	RevokeRefreshSessionByID(userID, sessionID, currentSessionID string) error
	RevokeOtherRefreshSessions(userID, currentSessionID string) (int64, error)
	ChangePassword(userID, currentPassword, newPassword, keepSessionID string) (revoked int64, err error)
	UpdateProfileSettings(userID string, update ProfileSettingsUpdate) error
}

// TelegramUserAuth holds the resolved user info and auth token for bot service communication.
type TelegramUserAuth struct {
	UserID          string `json:"user_id"`
	Email           string `json:"email"`
	AuthToken       string `json:"auth_token"`
	DefaultCurrency string `json:"default_currency"`
}

// TelegramServicer defines the contract for Telegram-related business logic.
type TelegramServicer interface {
	GetLinkByUserID(userID string) (*models.TelegramLink, error)
	GetLinkByTelegramID(telegramUserID int64) (*models.TelegramLink, error)
	GenerateLinkCode(userID string) (*models.TelegramLink, error)
	CompleteLink(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error
	UnlinkAccount(userID string) error
	RecordActivity(telegramUserID int64) error
	IsLinked(userID string) (bool, error)
	GetUserWithAuthToken(telegramUserID int64) (*TelegramUserAuth, error)
	// UpdateReminderSettings switches the reminder kinds on or off for the
	// user's link; nil fields are left unchanged (plan 023, 5.6).
	UpdateReminderSettings(userID string, settings ReminderSettings) (*models.TelegramLink, error)
}

// ReminderSettings holds the optional per-kind reminder switches of a Telegram
// link. A nil field means "don't change".
type ReminderSettings struct {
	CardDue    *bool
	LoanDue    *bool
	FDMaturity *bool
}

// RecordConsentInput holds the outcome of one consent decision (plan 022).
type RecordConsentInput struct {
	UserID     string
	ClientID   string
	ClientName string
	// Approved is the set of API scopes the user granted.
	Approved []string
	// Reviewed is every API scope the consent screen showed, approved or not.
	Reviewed []string
	// AutoAccept is the "remember this application" choice.
	AutoAccept bool
	// ClientHost is the host the authorization code is sent to ("" if unknown;
	// an empty value keeps the host already recorded).
	ClientHost string
}

// OAuthGrantServicer manages per-user, per-scope OAuth consent grants
// (plan 022). It backs the consent bridge (auto-accept decisions), the
// Connected apps settings (list / revoke) and the MCP server's write guard.
type OAuthGrantServicer interface {
	// Get returns the user's grant for clientID, including a revoked one, or
	// ErrOAuthClientNotFound.
	Get(userID, clientID string) (*models.OAuthClientGrant, error)
	// RecordConsent upserts the grant for (user, client), replacing its scopes
	// and clearing any earlier revocation.
	RecordConsent(in RecordConsentInput) (*models.OAuthClientGrant, error)
	// List returns the user's active (non-revoked) grants, most recent first.
	List(userID string) ([]models.OAuthClientGrant, error)
	// Revoke marks the user's grant for clientID revoked. It returns
	// ErrOAuthClientNotFound when there is no active grant.
	Revoke(userID, clientID string) error
	// Authorize checks that the user is active and holds an active grant for
	// clientID that includes scope, returning ErrOAuthScopeNotGranted otherwise.
	// It records the grant's last use.
	Authorize(userID, clientID, scope string) error
	// RevokeAll marks every active grant of the user revoked (used when the
	// user is deactivated) and returns how many were revoked.
	RevokeAll(userID string) (int64, error)
	// IsActive reports whether the user holds a non-revoked grant for clientID.
	// A missing grant is not active. The MCP server uses it to refuse reads
	// from disconnected (or never-granted) clients.
	IsActive(userID, clientID string) (bool, error)
	// Touch records that clientID was just used by the user (reads included).
	// It is throttled to one UPDATE per lastUsedInterval, in process and in SQL.
	Touch(userID, clientID string) error
	// SetClientHost records the host the client's authorization code goes to.
	// An empty host or a missing grant is a no-op.
	SetClientHost(userID, clientID, host string) error
}

// AccountUpdateFields holds optional fields for updating an account.
// Nil pointer means "don't change"; non-nil means "set to this value".
type AccountUpdateFields struct {
	Name          *string
	Description   *string
	IsActive      *bool
	IsPinned      *bool
	Broker        *string    // investment only
	AccountNumber *string    // investment only
	InterestRate  *float64   // credit_card and fixed_deposit
	DueDate       *time.Time // credit_card only
	CreditLimit   *int64     // credit_card only

	// DueDay (1-31) is the day of the month a credit card or loan falls due;
	// 0 clears it. MonthlyPayment (cents) is the loan instalment; debt only,
	// 0 clears it (plan 023, 5.3).
	DueDay         *int
	MonthlyPayment *int64

	// Fixed deposit mutable fields (plan 020, B4). Principal and StartDate are
	// immutable after creation and are intentionally absent here.
	MaturityDate    *time.Time              // fixed_deposit only
	PayoutType      *models.PayoutType      // fixed_deposit only
	PayoutFrequency *models.PayoutFrequency // fixed_deposit only; required when PayoutType=periodic
	LinkedAccountID *string                 // fixed_deposit only; pointer-to-nil clears the field
	Compounding     *models.Compounding     // fixed_deposit only
}

// CreateCreditCardParams holds the parameters for opening a credit card account.
type CreateCreditCardParams struct {
	Name         string
	Description  string
	Currency     string // defaults to the user's base currency
	CreditLimit  int64
	InterestRate float64
	DueDate      *time.Time
	// DueDay (1-31), 0 for none: the day of the month the statement falls due.
	DueDay int
	// CurrentBalance is the amount already owed, in cents (>= 0).
	CurrentBalance int64
}

// CreateDebtParams holds the parameters for creating a loan (debt) account.
type CreateDebtParams struct {
	Name        string
	Description string
	Currency    string // defaults to the user's base currency
	// AmountOwed (cents, >= 0) is recorded as an opening_balance row.
	AmountOwed   int64
	InterestRate float64
	// MonthlyPayment (cents, >= 0), 0 for none.
	MonthlyPayment int64
	// DueDay (1-31), 0 for none.
	DueDay int
}

// CreateFixedDepositParams holds the parameters for opening a new fixed deposit account.
type CreateFixedDepositParams struct {
	// Name is the display name of the FD account.
	Name string
	// Description is an optional description of the account.
	Description string
	// Currency is the ISO 4217 currency code (e.g. "USD"). Defaults to "USD" if empty.
	Currency string
	// SourceAccountID is the cash account from which the principal is transferred.
	SourceAccountID string
	// Principal is the amount to lock into the deposit, in cents. Must be > 0.
	Principal int64
	// InterestRate is the annual interest rate as a percentage (e.g. 3.5 for 3.5%).
	InterestRate float64
	// StartDate is the date the deposit opens.
	StartDate time.Time
	// MaturityDate is the date the deposit matures. Must be after StartDate.
	MaturityDate time.Time
	// PayoutType controls whether interest accumulates (cumulative) or is paid out
	// on a schedule (periodic).
	PayoutType models.PayoutType
	// PayoutFrequency is required when PayoutType is periodic.
	PayoutFrequency *models.PayoutFrequency
	// Compounding is the compounding frequency used to project the maturity value.
	Compounding models.Compounding
	// LinkedAccountID is the optional cash account for periodic payouts. Required
	// when PayoutType is periodic; optional for cumulative (defaults destination on close).
	LinkedAccountID *string
}

// CloseFixedDepositParams holds the parameters for closing (realizing) a fixed deposit.
type CloseFixedDepositParams struct {
	// DestinationAccountID is the cash account that receives the principal and interest.
	// If nil, the FD's LinkedAccountID is used.
	DestinationAccountID *string
	// InterestOverride, when non-nil, overrides the computed interest amount (cents).
	// Allows the user to adjust the realized interest in the UI.
	InterestOverride *int64
	// Penalty is an optional penalty amount in cents deducted from the interest payout
	// (e.g. for premature withdrawal). Defaults to 0. Never results in negative payout.
	Penalty int64
}

// AccountServicer defines the contract for account-related business logic.
type AccountServicer interface {
	CreateCashAccount(userID, name, description, currency string, initialBalance int64) (*models.Account, error)
	CreateInvestmentAccount(userID, name, description, currency, broker, accountNumber string) (*models.Account, error)
	CreateCreditCardAccount(userID, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time, currentBalance int64) (*models.Account, error)
	// CreateCreditCardAccountWithParams is CreateCreditCardAccount with the
	// recurring due day (plan 023, 5.3 / 5.6).
	CreateCreditCardAccountWithParams(userID string, params CreateCreditCardParams) (*models.Account, error)
	// CreateDebtAccount creates a loan: the amount owed is an opening_balance
	// row (never inverted), in one database transaction (plan 023, 5.3).
	CreateDebtAccount(userID string, params CreateDebtParams) (*models.Account, error)
	// CreateFixedDepositAccount opens a new fixed deposit, transferring principal from a
	// source cash account atomically (plan 020, B4).
	CreateFixedDepositAccount(userID string, params CreateFixedDepositParams) (*models.Account, error)
	// CloseFixedDeposit realizes a fixed deposit — returning principal and posting net
	// interest as income — and marks the deposit closed (plan 020, B4).
	CloseFixedDeposit(userID, accountID string, params CloseFixedDepositParams) (*models.Account, error)
	GetUserAccounts(userID string, page pagination.PageRequest, opts ...AccountListOptions) (*pagination.PageResponse[models.Account], error)
	GetAccountByID(userID, accountID string) (*models.Account, error)
	UpdateAccount(userID, accountID string, updates AccountUpdateFields) (*models.Account, error)
	// ApplyBalanceDelta applies a transaction's balance effect with a relative
	// update (balance = balance + delta) on accountID, whose type the caller
	// supplies. Income and reimbursement credit, expense debits; credit_card and
	// debt accounts are inverted. Safe under concurrency (plan 021, D10).
	ApplyBalanceDelta(tx *gorm.DB, accountID string, accountType models.AccountType, txnType models.TransactionType, amount int64) error
}

// CategoryServicer defines the contract for category-related business logic.
type CategoryServicer interface {
	CreateCategory(userID string, name string, categoryType models.CategoryType, description, icon, color string, parentID *string) (*models.Category, error)
	GetUserCategories(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error)
	GetUserCategoriesByType(userID string, categoryType models.CategoryType, page pagination.PageRequest) (*pagination.PageResponse[models.Category], error)
	GetCategoryByID(userID, categoryID string) (*models.Category, error)
	UpdateCategory(userID, categoryID string, name, description, icon, color string, parentID *string) (*models.Category, error)
	DeleteCategory(userID, categoryID string) error
	// EnsureInterestIncomeCategory idempotently finds or creates the user's
	// "Interest Income" income category (plan 020, B5). Used by the fixed-deposit
	// service to categorize periodic payouts and realization interest.
	EnsureInterestIncomeCategory(userID string) (*models.Category, error)
}

// TransactionUpdateFields holds optional fields for updating a transaction.
// Nil pointer means "don't change"; non-nil means "set to this value".
// CategoryID uses a double pointer: nil=no change, *nil=clear, *value=set.
type TransactionUpdateFields struct {
	AccountID   *string
	CategoryID  **string
	Type        *models.TransactionType
	Amount      *int64
	Description *string
	Date        *time.Time
	Notes       *string
}

// TransactionMeta carries the provenance of a new transaction: free-text notes
// and where the row came from. An empty Source is stored as "web".
// SourceClientID is the OAuth client id for source "mcp".
type TransactionMeta struct {
	Notes          string
	Source         models.TransactionSource
	SourceClientID string
}

// TransactionFilter holds optional filter parameters for listing transactions.
type TransactionFilter struct {
	FromDate   *time.Time
	ToDate     *time.Time
	Type       *models.TransactionType
	CategoryID *string
	MinAmount  *int64
	MaxAmount  *int64
	AccountID  *string
	// Search matches description or notes (case-insensitive substring) and, when
	// it parses as an amount such as "64.73", the amount in cents.
	Search *string
	// Shared keeps only rows that have (true) or lack (false) receivables.
	Shared *bool
	// HasAttachment keeps only rows that have (true) or lack (false) receipts.
	HasAttachment *bool
}

// SpendingByCategoryItem represents spending total for a single category.
type SpendingByCategoryItem struct {
	CategoryID    *string `json:"category_id"`
	CategoryName  string  `json:"category_name"`
	CategoryColor string  `json:"category_color"`
	CategoryIcon  string  `json:"category_icon"`
	Total         int64   `json:"total"`
}

// SpendingByCategory represents the full spending breakdown response.
type SpendingByCategory struct {
	Items      []SpendingByCategoryItem `json:"items"`
	TotalSpent int64                    `json:"total_spent"`
	FromDate   time.Time                `json:"from_date"`
	ToDate     time.Time                `json:"to_date"`
	// Currency is the currency of every amount (the user's base currency).
	Currency string `json:"currency"`
	// FXMissing is true when some spending had no exchange rate and was left out.
	FXMissing bool `json:"fx_missing"`
	// Unconverted lists, per currency, the native amounts left out of the totals.
	Unconverted []UnconvertedAmount `json:"unconverted"`
}

// UnconvertedAmount is a native-currency amount that could not be converted
// because no exchange rate was available. It is excluded from converted totals.
type UnconvertedAmount struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"` // minor units, in Currency
}

// Cashflow represents income and expense totals grouped by category for a date
// range, used to render the income-to-expense cashflow Sankey. Only "income" and
// "expense" transactions are aggregated; transfers and investments are excluded.
type Cashflow struct {
	Income        []SpendingByCategoryItem `json:"income"`
	Expenses      []SpendingByCategoryItem `json:"expenses"`
	TotalIncome   int64                    `json:"total_income"`   // cents
	TotalExpenses int64                    `json:"total_expenses"` // cents
	FromDate      time.Time                `json:"from_date"`
	ToDate        time.Time                `json:"to_date"`
	Currency      string                   `json:"currency"`    // the user's base currency
	FXMissing     bool                     `json:"fx_missing"`  // some amounts had no rate and are excluded
	Unconverted   []UnconvertedAmount      `json:"unconverted"` // native amounts excluded from the totals
}

// DailySpendingItem represents expense total for a single day.
type DailySpendingItem struct {
	Date  string `json:"date"`  // "2026-02-01" format
	Total int64  `json:"total"` // cents, in Currency
	// Currency is the user's base currency; FXMissing is true when some of the
	// day's spending had no exchange rate and is excluded from Total.
	Currency  string `json:"currency"`
	FXMissing bool   `json:"fx_missing"`
}

// MonthlySummaryItem represents income and expense totals for a single month.
type MonthlySummaryItem struct {
	Month    string `json:"month"`    // "2026-02" format
	Income   int64  `json:"income"`   // cents, in Currency
	Expenses int64  `json:"expenses"` // cents, in Currency
	// Currency is the user's base currency; FXMissing is true when some of the
	// month's amounts had no exchange rate and are excluded.
	Currency  string `json:"currency"`
	FXMissing bool   `json:"fx_missing"`
}

// DailySummaryItem represents income and expense totals for a single day.
type DailySummaryItem struct {
	Date     string `json:"date"`     // "2026-02-01" format
	Income   int64  `json:"income"`   // cents, in Currency
	Expenses int64  `json:"expenses"` // cents, in Currency
	// Currency is the user's base currency; FXMissing is true when some of the
	// day's amounts had no exchange rate and are excluded.
	Currency  string `json:"currency"`
	FXMissing bool   `json:"fx_missing"`
}

// TopExpenseItem represents a single high-value expense transaction.
type TopExpenseItem struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	AccountName    string    `json:"account_name"`
	CategoryID     *string   `json:"category_id"`
	CategoryName   string    `json:"category_name"`
	CategoryColor  string    `json:"category_color"`
	CategoryIcon   string    `json:"category_icon"`
	Amount         int64     `json:"amount"`          // cents, the full amount that left the account
	PersonalAmount *int64    `json:"personal_amount"` // cents, the user's own share; nil when not shared
	Description    string    `json:"description"`
	Date           time.Time `json:"date"`
	// Currency is the account's currency, the unit of Amount and PersonalAmount.
	Currency string `json:"currency"`
	// AmountBase is the effective amount (personal share, else Amount) converted
	// to the user's base currency at the expense's date; 0 when FXMissing.
	AmountBase int64 `json:"amount_base"`
	// FXMissing is true when no rate existed, so AmountBase could not be computed.
	FXMissing bool `json:"fx_missing"`
}

// TopExpenses represents the top-N expense transactions for a date range.
type TopExpenses struct {
	Items []TopExpenseItem `json:"items"`
	// TotalCount is the number of expenses with a non-zero effective amount in
	// the range (ignoring the limit), for count and average statistics.
	TotalCount int64     `json:"total_count"`
	FromDate   time.Time `json:"from_date"`
	ToDate     time.Time `json:"to_date"`
	// Currency is the user's base currency (the unit of each AmountBase).
	Currency string `json:"currency"`
	// FXMissing is true when some matching expenses had no rate; they rank after
	// every converted expense.
	FXMissing bool `json:"fx_missing"`
}

// TransactionServicer defines the contract for transaction-related business logic.
type TransactionServicer interface {
	CreateTransaction(userID, accountID string, categoryID *string, transactionType models.TransactionType, amount int64, description string, date time.Time, meta TransactionMeta) (*models.Transaction, error)
	CreateTransfer(userID, fromAccountID, toAccountID string, amount int64, toAmount *int64, description string, date time.Time, meta TransactionMeta) (*models.Transaction, error)
	GetAccountTransactions(userID, accountID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	GetUserTransactions(userID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	GetTransactionByID(userID, transactionID string) (*models.Transaction, error)
	UpdateTransaction(userID, transactionID string, updates TransactionUpdateFields) (*models.Transaction, error)
	DeleteTransaction(userID, transactionID string) error
	BulkUpdateTransactions(userID string, in BulkTransactionInput) (*BulkTransactionResult, error)
	GetSpendingByCategory(userID string, from, to time.Time) (*SpendingByCategory, error)
	GetCashflow(userID string, from, to time.Time) (*Cashflow, error)
	GetMonthlySummary(userID string, months int) ([]MonthlySummaryItem, error)
	GetDailySpending(userID string, from, to time.Time) ([]DailySpendingItem, error)
	GetDailySummary(userID string, from, to time.Time) ([]DailySummaryItem, error)
	GetTopExpenses(userID string, from, to time.Time, limit int, categoryID *string) (*TopExpenses, error)
	// Rule backfill/preview (plan 018). These live here because they read/write
	// the transactions table; the pure matcher is shared via the rule service.
	PreviewRuleMatches(userID string, conditions []RuleConditionInput) (*RuleMatchPreview, error)
	ApplyRule(userID, ruleID string, opts ApplyRuleOptions) (*ApplyRuleResult, error)
}

// RuleConditionInput is a single AND-ed condition supplied when creating/updating
// a rule or previewing matches.
type RuleConditionInput struct {
	Field     models.RuleField
	Operator  models.RuleOperator
	ValueText string
	AmountMin *int64
	AmountMax *int64
}

// RuleActionInput is a single action supplied when creating/updating a rule.
type RuleActionInput struct {
	ActionType models.RuleActionType
	CategoryID *string
	ValueText  string
}

// CreateRuleInput holds the fields for creating a rule.
type CreateRuleInput struct {
	Name       string
	Priority   int
	IsActive   *bool
	Conditions []RuleConditionInput
	Actions    []RuleActionInput
}

// UpdateRuleInput holds the optional fields for updating a rule. Nil pointers and
// nil slices mean "leave unchanged"; a non-nil Conditions/Actions slice replaces
// the rule's children wholesale.
type UpdateRuleInput struct {
	Name       *string
	Priority   *int
	IsActive   *bool
	Conditions []RuleConditionInput
	Actions    []RuleActionInput
}

// RuleApplyScope selects which existing transactions a backfill considers.
type RuleApplyScope string

const (
	RuleApplyScopeUncategorized RuleApplyScope = "uncategorized"
	RuleApplyScopeAll           RuleApplyScope = "all"
)

// ApplyRuleOptions configures a rule backfill over existing transactions.
type ApplyRuleOptions struct {
	Scope     RuleApplyScope
	Overwrite bool // when Scope=all, overwrite an existing (non-nil) category
	DryRun    bool // when true, count/sample only; write nothing
}

// ApplyRuleResult reports the outcome of a backfill.
type ApplyRuleResult struct {
	Count   int                  `json:"count"`   // transactions the rule would categorize
	Applied int                  `json:"applied"` // transactions actually updated (0 on dry run)
	Sample  []models.Transaction `json:"sample"`  // small preview of affected transactions
}

// RuleMatchPreview reports how many existing transactions match a set of
// (unsaved) conditions, for the "matches N existing" UI.
type RuleMatchPreview struct {
	Count  int                  `json:"count"`
	Sample []models.Transaction `json:"sample"`
}

// RuleServicer defines the contract for transaction-rule business logic (plan 018).
type RuleServicer interface {
	CreateRule(userID string, in CreateRuleInput) (*models.TransactionRule, error)
	GetRule(userID, ruleID string) (*models.TransactionRule, error)
	ListRules(userID string) ([]models.TransactionRule, error)
	UpdateRule(userID, ruleID string, in UpdateRuleInput) (*models.TransactionRule, error)
	DeleteRule(userID, ruleID string) error
	ReorderRules(userID string, ruleIDs []string) ([]models.TransactionRule, error)
	// ResolveForUser loads the user's active rules and returns the category the
	// input transaction should receive (if any). Used by transaction creation.
	ResolveForUser(userID string, in RuleInput) (RuleResult, error)
	// SuggestRules proposes rules from repeated categorizations no rule covers.
	SuggestRules(userID string) ([]RuleSuggestion, error)
}

// BudgetProgress contains spending vs budget data for a budget's current period.
type BudgetProgress struct {
	BudgetID   string  `json:"budget_id"`
	Budgeted   int64   `json:"budgeted"`
	Spent      int64   `json:"spent"`
	Remaining  int64   `json:"remaining"`
	Percentage float64 `json:"percentage"`
	// Currency is the budget's currency (the unit of every amount above).
	Currency string `json:"currency"`
	// FXMissing is true when some spending had no exchange rate and is excluded from Spent.
	FXMissing bool `json:"fx_missing"`
}

// BudgetOption sets an optional budget attribute on create or update.
type BudgetOption func(*budgetOptions)

type budgetOptions struct {
	currency string
}

// WithBudgetCurrency sets the budget's ISO 4217 currency. On create the default
// is the user's base currency; on update an empty value leaves it unchanged.
func WithBudgetCurrency(code string) BudgetOption {
	return func(o *budgetOptions) { o.currency = code }
}

// BudgetServicer defines the contract for budget-related business logic.
type BudgetServicer interface {
	CreateBudget(userID, categoryID string, name string, amount int64, period models.BudgetPeriod, opts ...BudgetOption) (*models.Budget, error)
	GetUserBudgets(userID string, page pagination.PageRequest, isActive *bool, period *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error)
	GetBudgetByID(userID, budgetID string) (*models.Budget, error)
	UpdateBudget(userID, budgetID string, name string, amount *int64, period *models.BudgetPeriod, isActive *bool, opts ...BudgetOption) (*models.Budget, error)
	DeleteBudget(userID, budgetID string) error
	GetBudgetProgress(userID, budgetID string) (*BudgetProgress, error)
	GetActiveBudgetsProgress(userID string) ([]BudgetProgress, error)
}

// PortfolioSummary contains aggregated portfolio data across all investment accounts.
type PortfolioSummary struct {
	TotalValue            int64                            `json:"total_value"`
	TotalCostBasis        int64                            `json:"total_cost_basis"`
	TotalGainLoss         int64                            `json:"total_gain_loss"`
	GainLossPct           float64                          `json:"gain_loss_pct"`
	TotalRealizedGainLoss int64                            `json:"total_realized_gain_loss"`
	HoldingsByType        map[models.AssetType]TypeSummary `json:"holdings_by_type"`
	// TotalPriceReturn and TotalFXReturn split the unrealized gain
	// (TotalValue - TotalCostBasis): the native gain at today's rate, and the
	// currency effect on the cost. They sum to TotalGainLoss within a cent per holding.
	TotalPriceReturn int64 `json:"total_price_return"`
	TotalFXReturn    int64 `json:"total_fx_return"`
	// Currency is the user's base currency, which every value above is in
	// (cost basis and realized gain/loss are the frozen base-currency figures).
	Currency string `json:"currency"`
	// FXStale is true when any FX quote used was stale.
	FXStale bool `json:"fx_stale"`
	// Unconverted lists holdings that could not be valued for lack of an FX
	// rate, as native value summed per currency. They are left out of the totals.
	Unconverted []UnconvertedValue `json:"unconverted"`
}

// UnconvertedValue is the native market value of holdings that have no FX rate
// to the base currency.
type UnconvertedValue struct {
	Currency string `json:"currency"`
	Value    int64  `json:"value"`
}

// TypeSummary contains summary data for a single asset type.
type TypeSummary struct {
	Value int64 `json:"value"`
	Count int   `json:"count"`
}

// AddInvestmentParams are the inputs of InvestmentServicer.AddInvestment.
// PurchasePrice and Fee are in the security's currency.
type AddInvestmentParams struct {
	AccountID     string
	SecurityID    string
	Quantity      float64
	PurchasePrice int64
	WalletAddress string
	Date          *time.Time // optional, defaults to now
	Fee           int64
	Notes         string // optional, defaults to "Initial purchase"
	// FXRate optionally overrides the security-currency to base rate of the
	// trade, as a decimal string.
	FXRate string
}

// BuyParams are the inputs of InvestmentServicer.RecordBuy. PricePerUnit and
// Fee are in the security's currency.
type BuyParams struct {
	Date         time.Time
	Quantity     float64
	PricePerUnit int64
	Fee          int64
	Notes        string
	// FundingAccountID optionally names the cash account the cost is paid from.
	FundingAccountID string
	// FundingAmount is the amount debited, in the funding account's currency.
	// Required when that differs from the security's currency (0 = not given);
	// otherwise it must be omitted or equal the trade total.
	FundingAmount int64
	// FXRate optionally overrides the trade's security-currency to base rate.
	FXRate string
}

// SellParams are the inputs of InvestmentServicer.RecordSell. PricePerUnit and
// Fee are in the security's currency.
type SellParams struct {
	Date         time.Time
	Quantity     float64
	PricePerUnit int64
	Fee          int64
	Notes        string
	// DepositAccountID optionally names the cash account the net proceeds go to.
	DepositAccountID string
	// DepositAmount is the amount credited, in the deposit account's currency.
	// Required when that differs from the security's currency (0 = not given).
	DepositAmount int64
	// FXRate optionally overrides the trade's security-currency to base rate.
	FXRate string
}

// DividendParams are the inputs of InvestmentServicer.RecordDividend. Amount is
// the gross dividend and Fee the withholding tax and fees, both in the
// security's currency.
type DividendParams struct {
	Date         time.Time
	Amount       int64
	Fee          int64
	DividendType string
	Notes        string
	// DepositAccountID optionally names the cash account the NET dividend
	// (Amount - Fee) is credited to.
	DepositAccountID string
	// DepositAmount is the net amount credited, in the deposit account's
	// currency. Required when that differs from the security's currency.
	DepositAmount int64
	// FXRate optionally overrides the security-currency to base rate.
	FXRate string
}

// InvestmentServicer defines the contract for investment-related business logic.
type InvestmentServicer interface {
	AddInvestment(userID string, p AddInvestmentParams) (*models.Investment, error)
	GetAllInvestments(userID string, status InvestmentStatus, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error)
	GetAccountInvestments(userID, accountID string, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error)
	GetInvestmentByID(userID, investmentID string) (*models.Investment, error)
	GetPortfolio(userID string) (*PortfolioSummary, error)
	RecordBuy(userID, investmentID string, p BuyParams) (*models.InvestmentTransaction, error)
	RecordSell(userID, investmentID string, p SellParams) (*models.InvestmentTransaction, error)
	RecordDividend(userID, investmentID string, p DividendParams) (*models.InvestmentTransaction, error)
	RecordSplit(userID, investmentID string, date time.Time, splitRatio float64, notes string) (*models.InvestmentTransaction, error)
	GetInvestmentTransactions(userID, investmentID string, page pagination.PageRequest) (*pagination.PageResponse[models.InvestmentTransaction], error)
}

// CurrencyPair names an exchange-rate pair: one unit of Base is worth Rate
// units of Quote. Codes are upper-case ISO 4217.
type CurrencyPair struct{ Base, Quote string }

// FX quote paths, describing how an FXQuote was resolved.
const (
	// FXPathIdentity is a quote of a currency against itself.
	FXPathIdentity = "identity"
	// FXPathDirect is a stored base→quote observation.
	FXPathDirect = "direct"
	// FXPathInverse is the inverse of a stored quote→base observation.
	FXPathInverse = "inverse"
	// FXPathPivotUSD is the product of two legs through USD.
	FXPathPivotUSD = "pivot:USD"
)

// FXQuote is an exchange rate resolved for a point in time.
type FXQuote struct {
	Base, Quote string
	Rate        fx.Rate
	// RecordedAt is when the rate was observed; for a pivot, the older of the two legs.
	RecordedAt time.Time
	// Stale is true when at - RecordedAt exceeds the configured maximum staleness.
	Stale bool
	// Path is one of the FXPath* constants.
	Path string
}

// FXRateInput is one rate to record.
type FXRateInput struct {
	Base, Quote string
	Rate        fx.Rate
	RecordedAt  time.Time
	// Source names the provider; empty means "yahoo".
	Source string
}

// FXServicer defines the contract for exchange-rate lookups and recording.
type FXServicer interface {
	// Rate resolves from→to as of at (identity, direct, inverse, then USD pivot).
	// It returns ErrFXRateUnavailable when none exists.
	Rate(from, to string, at time.Time) (*FXQuote, error)
	// Rates resolves many pairs in a bounded number of queries. Pairs with no
	// rate are absent from the map (no error).
	Rates(pairs []CurrencyPair, at time.Time) (map[CurrencyPair]*FXQuote, error)
	// RatesByDay resolves many pairs for every UTC day in [start, end]
	// (inclusive) in a bounded number of queries. Each day's quote is the one
	// Rate(pair, end of that day) would return. Days with no rate are absent
	// from the inner map; keys are "YYYY-MM-DD".
	RatesByDay(pairs []CurrencyPair, start, end time.Time) (map[CurrencyPair]map[string]*FXQuote, error)
	// RecordRates upserts rates on (base, quote, recorded_at) and returns the count written.
	RecordRates(rates []FXRateInput) (int, error)
	// ListRates returns the stored observations of the direct pair in [from, to], oldest first.
	ListRates(base, quote string, from, to time.Time) ([]models.FXRate, error)
	// NeededPairs lists the pairs the oracle should publish and the users' base currencies.
	NeededPairs() (pairs []CurrencyPair, baseCurrencies []string, err error)
	// IsSupportedCurrency reports whether code can be used as a base currency.
	IsSupportedCurrency(code string, at time.Time) bool
}

// SecurityPriceInput represents a single price entry for bulk recording.
type SecurityPriceInput struct {
	SecurityID string    `json:"security_id"`
	Price      int64     `json:"price"`
	RecordedAt time.Time `json:"recorded_at"`
	// Currency is required; an entry without one is rejected (currency_required).
	Currency string `json:"currency,omitempty"`
}

// RejectedPrice describes a price entry that RecordPrices skipped.
type RejectedPrice struct {
	Index            int    `json:"index"`
	SecurityID       string `json:"security_id"`
	Currency         string `json:"currency,omitempty"`
	ExpectedCurrency string `json:"expected_currency,omitempty"`
	Reason           string `json:"reason"`
}

// Reasons a price entry can be rejected.
const (
	// PriceRejectedUnknownSecurity means the security does not exist.
	PriceRejectedUnknownSecurity = "unknown_security"
	// PriceRejectedCurrencyMismatch means the entry's currency differs from the security's.
	PriceRejectedCurrencyMismatch = "currency_mismatch"
	// PriceRejectedCurrencyRequired means the entry carries no currency.
	PriceRejectedCurrencyRequired = "currency_required"
)

// RecordPricesResult is the outcome of RecordPrices.
type RecordPricesResult struct {
	// Recorded is the number of rows actually inserted.
	Recorded int `json:"recorded"`
	// Rejected lists skipped entries; never nil.
	Rejected []RejectedPrice `json:"rejected"`
}

// SecurityUpdate holds the optional fields of a security correction; nil
// fields are left unchanged.
type SecurityUpdate struct {
	Currency       *string
	Exchange       *string
	ProviderSymbol *string
	Name           *string
	// ResetPrices deletes the security's prices; it requires a currency change.
	ResetPrices bool
}

// SecurityUpdateResult is the outcome of UpdateSecurity.
type SecurityUpdateResult struct {
	Security      *models.Security `json:"security"`
	PricesDeleted int64            `json:"prices_deleted"`
}

// SecurityServicer defines the interface for security-related operations.
type SecurityServicer interface {
	CreateSecurity(symbol, name string, assetType models.AssetType, currency, exchange string, extraFields map[string]interface{}) (*models.Security, error)
	GetSecurityByID(id string) (*models.Security, error)
	ListSecurities(search string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error)
	ListAllSecurities() ([]models.Security, error)
	RecordPrices(prices []SecurityPriceInput) (*RecordPricesResult, error)
	UpdateSecurity(id string, update SecurityUpdate) (*SecurityUpdateResult, error)
	GetPriceHistory(securityID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error)
}

// PortfolioSnapshotServicer defines the interface for portfolio snapshot operations.
type PortfolioSnapshotServicer interface {
	ComputeAndRecordSnapshots(recordedAt time.Time) (int, error)
	GetSnapshots(userID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error)
	GetGroupedSnapshots(userID string, from, to time.Time, groupBy string) ([]models.PortfolioSnapshot, error)
}

// ImportServicer defines the contract for statement imports (plan 024, phase 3b).
type ImportServicer interface {
	// ImportIBKR turns Interactive Brokers Activity Statement CSVs into the
	// IBKR holdings and the USD cash ledger. A dry run (p.Apply false) returns
	// the report without writing; p.Apply writes everything in one DB
	// transaction, and is refused with IMPORT_INVALID when the report has
	// errors or a failed quantity checkpoint. The files are only read in memory.
	ImportIBKR(userID string, p IBKRImportParams, files []io.Reader) (*IBKRImportReport, error)
}

// AuditServicer defines the contract for audit logging.
type AuditServicer interface {
	// Log records an event. Without opts the row's source is "web".
	Log(userID, action, resourceType, resourceID, ipAddress string, changes map[string]interface{}, opts ...LogOpts)
}

// LogOpts carries optional provenance for an audit row. The zero value (and
// omitting it) records a "web" event with no client.
type LogOpts struct {
	// Source is where the event came from: "web" (default) or "mcp".
	Source string
	// ClientID is the OAuth client behind an "mcp" event.
	ClientID string
}

// AttachmentLimits holds the per-request caps enforced by the attachment
// service. It mirrors config.AttachmentConfig without coupling the services
// package to the config package.
type AttachmentLimits struct {
	MaxUploadBytes      int64
	MaxAttachmentsPerTx int
}

// AttachmentServicer defines the contract for transaction receipt attachments
// (plan 017). All operations are user-scoped; ownership is enforced on every
// read, write, and delete.
type AttachmentServicer interface {
	// Upload sniffs, sanitizes (EXIF strip + bomb defense), stores the bytes,
	// and persists metadata for a receipt attached to the user's transaction.
	Upload(userID, txID, fileName, declaredType string, size int64, data io.Reader) (*models.TransactionAttachment, error)
	// List returns the attachment metadata for a user's transaction.
	List(userID, txID string) ([]models.TransactionAttachment, error)
	// Open returns an attachment's metadata and a byte stream the caller must
	// Close. The attachment must belong to both the user and the given
	// transaction; ownership is checked before any bytes are read.
	Open(userID, txID, attachmentID string) (*models.TransactionAttachment, io.ReadCloser, error)
	// Delete removes the metadata row and the stored object. The attachment must
	// belong to both the user and the given transaction.
	Delete(userID, txID, attachmentID string) error
}

// ShareMode selects how a shared expense is divided among counterparties
// (plan 021, D5).
type ShareMode string

// Share modes accepted by CreateSharedExpense.
const (
	// ShareModeEqual splits the amount equally between the user and each counterparty.
	ShareModeEqual ShareMode = "equal"
	// ShareModeCustom takes an explicit amount for every counterparty.
	ShareModeCustom ShareMode = "custom"
	// ShareModeFull makes a single counterparty owe the entire amount (personal share 0).
	ShareModeFull ShareMode = "full"
)

// SharedCounterpartyInput is one counterparty of a new shared expense. Amount
// must be nil for the equal and full modes and set for the custom mode.
type SharedCounterpartyInput struct {
	Name   string
	Amount *int64
}

// CreateSharedExpenseInput holds the fields for creating an expense together
// with its receivables.
type CreateSharedExpenseInput struct {
	AccountID      string
	CategoryID     *string
	Amount         int64
	Description    string
	Date           time.Time // zero means now
	Kind           models.ReceivableKind
	Mode           ShareMode
	Counterparties []SharedCounterpartyInput
	Meta           TransactionMeta
}

// ReceivableShareInput is one entry of the D7 receivables diff. A non-nil ID
// updates that receivable in place; a nil ID inserts a new one.
type ReceivableShareInput struct {
	ID     *string
	Name   string
	Amount int64
}

// UpdateReceivablesInput is the D7 edit payload. Receivables not listed (and
// unpaid) are removed. Kind, when set, applies to all of the expense's receivables.
type UpdateReceivablesInput struct {
	Kind        *models.ReceivableKind
	Receivables []ReceivableShareInput
}

// SettleInput holds the fields for recording a repayment against a receivable.
type SettleInput struct {
	AccountID         string
	Amount            int64
	Date              time.Time // zero means now
	Description       string    // empty means the default "Repayment from {name}: {expense}"
	WriteOffRemainder bool
	// AccountAmount is the amount credited to the account, in the account's
	// currency. Required when it differs from the receivable's currency; must
	// be nil or equal to Amount otherwise (plan 024, phase 4).
	AccountAmount *int64
}

// ReceivableFilter holds the optional filters for listing receivables. Status
// is a receivable status or "open" (pending or partially paid); Kind is a
// receivable kind; CounterpartyKey is a normalized counterparty key. Empty
// strings mean "no filter".
type ReceivableFilter struct {
	Status          models.ReceivableStatus
	Kind            models.ReceivableKind
	CounterpartyKey string
}

// ReceivableStatusOpen is the pseudo-status matching pending and partially paid
// receivables. It is only meaningful in ReceivableFilter.Status; it is never stored.
const ReceivableStatusOpen models.ReceivableStatus = "open"

// ReceivableKindTotal is the open outstanding total for one receivable kind.
type ReceivableKindTotal struct {
	Kind        models.ReceivableKind `json:"kind"`
	Outstanding int64                 `json:"outstanding"` // cents
	OpenCount   int                   `json:"open_count"`
}

// ReceivableCounterpartyTotal is the open outstanding total for one counterparty.
type ReceivableCounterpartyTotal struct {
	CounterpartyKey  string    `json:"counterparty_key"`
	CounterpartyName string    `json:"counterparty_name"`
	Outstanding      int64     `json:"outstanding"` // cents
	OpenCount        int       `json:"open_count"`
	OldestDate       time.Time `json:"oldest_date"` // date of the oldest open expense
}

// ReceivableSummary aggregates the user's open receivables.
type ReceivableSummary struct {
	TotalOutstanding int64                         `json:"total_outstanding"` // cents, in Currency (the user's base)
	Currency         string                        `json:"currency"`          // currency of the converted totals (base)
	FXMissing        bool                          `json:"fx_missing"`        // some outstanding amounts had no rate and are left out
	ByCurrency       []ReceivableCurrencyTotal     `json:"by_currency"`       // native outstanding per receivable currency
	ByKind           []ReceivableKindTotal         `json:"by_kind"`
	ByCounterparty   []ReceivableCounterpartyTotal `json:"by_counterparty"`
}

// ReceivableCurrencyTotal is the open outstanding total in one receivable
// currency, in that currency (not converted).
type ReceivableCurrencyTotal struct {
	Currency    string `json:"currency"`
	Outstanding int64  `json:"outstanding"` // cents of Currency
}

// CounterpartySuggestion is an autocomplete entry: a normalized key and the
// display name last used for it.
type CounterpartySuggestion struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ReceivableServicer defines the contract for receivables (plan 021): money
// that left an account, is expected back and is not spending. All operations
// are user-scoped. Mutations run in one DB transaction under the D10 lock order
// (expense, receivables by id, accounts by id).
type ReceivableServicer interface {
	// CreateSharedExpense creates an expense and its receivables atomically and
	// sets the expense's personal_amount.
	CreateSharedExpense(userID string, in CreateSharedExpenseInput) (*models.Transaction, error)
	// UpdateReceivables applies the D7 diff to an expense's receivables.
	UpdateReceivables(userID, transactionID string, in UpdateReceivablesInput) (*models.Transaction, error)
	// Settle records a repayment: it creates a reimbursement transaction in the
	// chosen account and raises the receivable's paid_amount.
	Settle(userID, receivableID string, in SettleInput) (*models.Receivable, *models.ReceivableSettlement, error)
	// DeleteSettlement reverses a repayment exactly once.
	DeleteSettlement(userID, settlementID string) (*models.Receivable, error)
	// WriteOff forgives amount (nil means the whole outstanding balance).
	WriteOff(userID, receivableID string, amount *int64) (*models.Receivable, error)
	// UndoWriteOff resets the receivable's written-off amount to zero.
	UndoWriteOff(userID, receivableID string) (*models.Receivable, error)
	// ListReceivables returns a page of receivables, newest expense first, each with its expense attached.
	ListReceivables(userID string, page pagination.PageRequest, filter ReceivableFilter) (*pagination.PageResponse[models.Receivable], error)
	// GetReceivable returns one receivable with its settlements and expense.
	GetReceivable(userID, receivableID string) (*models.Receivable, error)
	// Summary aggregates open receivables per kind and per counterparty.
	Summary(userID string) (*ReceivableSummary, error)
	// ListCounterparties returns up to 20 counterparty suggestions matching q, most recently used first.
	ListCounterparties(userID, q string) ([]CounterpartySuggestion, error)
	// RenameCounterparty renames (or merges) every receivable with fromKey and returns the number updated.
	RenameCounterparty(userID, fromKey, toName string) (int64, error)
}
