package services

import (
	"io"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
)

// UserServicer defines the contract for user-related business logic.
type UserServicer interface {
	CreateUser(email, password, firstName, lastName string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)
	VerifyPassword(user *models.User, password string) bool
	AttemptLogin(email, password string) (*models.User, error)
	StoreRefreshTokenHash(userID, tokenHash string) error
	GetRefreshTokenHash(userID string) (string, error)
	UpdateHideBalances(userID string, hide bool) error
	UpdateDefaultAccount(userID string, accountID *string) error
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
}

// TrustedClientServicer defines the contract for managing OAuth clients that a
// user has approved via trust-on-first-use consent.
type TrustedClientServicer interface {
	IsTrusted(clientID string) (bool, error)
	Trust(clientID, name string) (*models.TrustedOAuthClient, error)
	ListTrusted() ([]models.TrustedOAuthClient, error)
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

	// Fixed deposit mutable fields (plan 020, B4). Principal and StartDate are
	// immutable after creation and are intentionally absent here.
	MaturityDate    *time.Time              // fixed_deposit only
	PayoutType      *models.PayoutType      // fixed_deposit only
	PayoutFrequency *models.PayoutFrequency // fixed_deposit only; required when PayoutType=periodic
	LinkedAccountID *string                 // fixed_deposit only; pointer-to-nil clears the field
	Compounding     *models.Compounding     // fixed_deposit only
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
	CreateCreditCardAccount(userID, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error)
	// CreateFixedDepositAccount opens a new fixed deposit, transferring principal from a
	// source cash account atomically (plan 020, B4).
	CreateFixedDepositAccount(userID string, params CreateFixedDepositParams) (*models.Account, error)
	// CloseFixedDeposit realizes a fixed deposit — returning principal and posting net
	// interest as income — and marks the deposit closed (plan 020, B4).
	CloseFixedDeposit(userID, accountID string, params CloseFixedDepositParams) (*models.Account, error)
	GetUserAccounts(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error)
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
}

// DailySpendingItem represents expense total for a single day.
type DailySpendingItem struct {
	Date  string `json:"date"`  // "2026-02-01" format
	Total int64  `json:"total"` // cents
}

// MonthlySummaryItem represents income and expense totals for a single month.
type MonthlySummaryItem struct {
	Month    string `json:"month"`    // "2026-02" format
	Income   int64  `json:"income"`   // cents
	Expenses int64  `json:"expenses"` // cents
}

// DailySummaryItem represents income and expense totals for a single day.
type DailySummaryItem struct {
	Date     string `json:"date"`     // "2026-02-01" format
	Income   int64  `json:"income"`   // cents
	Expenses int64  `json:"expenses"` // cents
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
}

// TopExpenses represents the top-N expense transactions for a date range.
type TopExpenses struct {
	Items []TopExpenseItem `json:"items"`
	// TotalCount is the number of expenses with a non-zero effective amount in
	// the range (ignoring the limit), for count and average statistics.
	TotalCount int64     `json:"total_count"`
	FromDate   time.Time `json:"from_date"`
	ToDate     time.Time `json:"to_date"`
}

// TransactionServicer defines the contract for transaction-related business logic.
type TransactionServicer interface {
	CreateTransaction(userID, accountID string, categoryID *string, transactionType models.TransactionType, amount int64, description string, date time.Time) (*models.Transaction, error)
	CreateTransfer(userID, fromAccountID, toAccountID string, amount int64, description string, date time.Time) (*models.Transaction, error)
	GetAccountTransactions(userID, accountID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	GetUserTransactions(userID string, page pagination.PageRequest, filter TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	GetTransactionByID(userID, transactionID string) (*models.Transaction, error)
	UpdateTransaction(userID, transactionID string, updates TransactionUpdateFields) (*models.Transaction, error)
	DeleteTransaction(userID, transactionID string) error
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
}

// BudgetProgress contains spending vs budget data for a budget's current period.
type BudgetProgress struct {
	BudgetID   string  `json:"budget_id"`
	Budgeted   int64   `json:"budgeted"`
	Spent      int64   `json:"spent"`
	Remaining  int64   `json:"remaining"`
	Percentage float64 `json:"percentage"`
}

// BudgetServicer defines the contract for budget-related business logic.
type BudgetServicer interface {
	CreateBudget(userID, categoryID string, name string, amount int64, period models.BudgetPeriod) (*models.Budget, error)
	GetUserBudgets(userID string, page pagination.PageRequest, isActive *bool, period *models.BudgetPeriod) (*pagination.PageResponse[models.Budget], error)
	GetBudgetByID(userID, budgetID string) (*models.Budget, error)
	UpdateBudget(userID, budgetID string, name string, amount *int64, period *models.BudgetPeriod, isActive *bool) (*models.Budget, error)
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
}

// TypeSummary contains summary data for a single asset type.
type TypeSummary struct {
	Value int64 `json:"value"`
	Count int   `json:"count"`
}

// InvestmentServicer defines the contract for investment-related business logic.
type InvestmentServicer interface {
	AddInvestment(userID, accountID, securityID string, quantity float64, purchasePrice int64, walletAddress string, date *time.Time, fee int64, notes string) (*models.Investment, error)
	GetAllInvestments(userID string, status InvestmentStatus, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error)
	GetAccountInvestments(userID, accountID string, page pagination.PageRequest) (*pagination.PageResponse[models.Investment], error)
	GetInvestmentByID(userID, investmentID string) (*models.Investment, error)
	GetPortfolio(userID string) (*PortfolioSummary, error)
	RecordBuy(userID, investmentID string, date time.Time, quantity float64, pricePerUnit int64, fee int64, notes string, fundingAccountID string) (*models.InvestmentTransaction, error)
	RecordSell(userID, investmentID string, date time.Time, quantity float64, pricePerUnit int64, fee int64, notes string, depositAccountID string) (*models.InvestmentTransaction, error)
	RecordDividend(userID, investmentID string, date time.Time, amount int64, dividendType, notes string) (*models.InvestmentTransaction, error)
	RecordSplit(userID, investmentID string, date time.Time, splitRatio float64, notes string) (*models.InvestmentTransaction, error)
	GetInvestmentTransactions(userID, investmentID string, page pagination.PageRequest) (*pagination.PageResponse[models.InvestmentTransaction], error)
}

// SecurityPriceInput represents a single price entry for bulk recording.
type SecurityPriceInput struct {
	SecurityID string    `json:"security_id"`
	Price      int64     `json:"price"`
	RecordedAt time.Time `json:"recorded_at"`
}

// SecurityServicer defines the interface for security-related operations.
type SecurityServicer interface {
	CreateSecurity(symbol, name string, assetType models.AssetType, currency, exchange string, extraFields map[string]interface{}) (*models.Security, error)
	GetSecurityByID(id string) (*models.Security, error)
	ListSecurities(search string, page pagination.PageRequest) (*pagination.PageResponse[models.Security], error)
	ListAllSecurities() ([]models.Security, error)
	RecordPrices(prices []SecurityPriceInput) (int, error)
	GetPriceHistory(securityID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.SecurityPrice], error)
}

// PortfolioSnapshotServicer defines the interface for portfolio snapshot operations.
type PortfolioSnapshotServicer interface {
	ComputeAndRecordSnapshots(recordedAt time.Time) (int, error)
	GetSnapshots(userID string, from, to time.Time, page pagination.PageRequest) (*pagination.PageResponse[models.PortfolioSnapshot], error)
	GetGroupedSnapshots(userID string, from, to time.Time, groupBy string) ([]models.PortfolioSnapshot, error)
}

// AuditServicer defines the contract for audit logging.
type AuditServicer interface {
	Log(userID, action, resourceType, resourceID, ipAddress string, changes map[string]interface{})
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
	TotalOutstanding int64                         `json:"total_outstanding"` // cents
	ByKind           []ReceivableKindTotal         `json:"by_kind"`
	ByCounterparty   []ReceivableCounterpartyTotal `json:"by_counterparty"`
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
