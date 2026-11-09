import type {
  Account,
  Attachment,
  Budget,
  BudgetPeriod,
  BudgetProgress,
  Category,
  Compounding,
  Investment,
  InvestmentTransaction,
  PayoutFrequency,
  PayoutType,
  PortfolioSummary,
  Security,
  Transaction,
  TransactionRule,
  RuleField,
  RuleOperator,
  RuleActionType,
  User,
  TransactionType,
  CategoryType,
  Receivable,
  ReceivableKind,
  ReceivableSettlement,
  ReceivableStatus,
  ShareMode,
} from "./models";

// Pagination
export interface PageResponse<T> {
  data: T[];
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}

export interface PaginationParams {
  page?: number;
  page_size?: number;
}

// Error response (matches backend { error: { code, message } })
export interface ApiError {
  error: {
    code: string;
    message: string;
    /** Machine-readable context, e.g. FX_AMOUNT_REQUIRED's suggested_amount. */
    details?: Record<string, unknown>;
  };
}

// Auth requests
export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  first_name?: string;
  last_name?: string;
  /** ISO 4217 reporting currency chosen at sign-up; defaults to MYR. */
  base_currency?: string;
}

// The refresh token travels in the HttpOnly `kuberan_refresh` cookie. The body
// field is only sent once, to migrate a session stored before the cookie.
export interface RefreshRequest {
  refresh_token?: string;
}

// Auth response (login, register). The refresh token is set as an HttpOnly
// cookie and is no longer part of the body.
export interface AuthResponse {
  access_token: string;
  user: User;
}

// POST /auth/refresh response. `user` lets the app bootstrap a session in one
// call; it is optional so an older API (without it) still works.
export interface RefreshResponse {
  access_token: string;
  user?: User;
}

// Profile response
export interface ProfileResponse {
  user: User;
}

// Request to update profile settings (PATCH /profile)
export interface UpdateProfileSettingsRequest {
  hide_balances?: boolean;
  default_account_id?: string | null; // null clears the default
  /** ISO 4217 reporting currency; 400 UNSUPPORTED_CURRENCY if it can't be converted. */
  base_currency?: string;
}

// Single-item response wrappers (backend wraps single items in a key)
export interface AccountResponse {
  account: Account;
}

export interface TransactionResponse {
  transaction: Transaction;
}

export interface CategoryResponse {
  category: Category;
}

export interface DeleteResponse {
  message: string;
}

// Attachment responses (receipt uploads). Upload returns the single created
// item; list returns the metadata array. Bytes are fetched separately as a Blob.
export interface AttachmentResponse {
  attachment: Attachment;
}

export interface AttachmentsResponse {
  attachments: Attachment[];
}

// Account requests
export interface CreateCashAccountRequest {
  name: string;
  description?: string;
  currency?: string; // ISO 4217, defaults to USD
  initial_balance?: number; // cents, >= 0
}

export interface CreateInvestmentAccountRequest {
  name: string;
  description?: string;
  currency?: string; // ISO 4217
  broker?: string;
  account_number?: string;
}

export interface CreateCreditCardAccountRequest {
  name: string;
  description?: string;
  currency?: string;
  credit_limit?: number;
  /** Amount currently owed on the card, in cents (>= 0). */
  current_balance?: number;
  interest_rate?: number;
  due_date?: string;
}

/** POST /api/v1/accounts/fixed-deposit */
export interface CreateFixedDepositAccountRequest {
  name: string;
  description?: string;
  currency?: string; // ISO 4217
  source_account_id: string; // UUIDv7 - cash account that funds the deposit
  principal: number; // cents, > 0
  interest_rate?: number; // annual %, float
  start_date: string; // "YYYY-MM-DD"
  maturity_date: string; // "YYYY-MM-DD", must be > start_date
  payout_type: PayoutType;
  payout_frequency?: PayoutFrequency; // required when payout_type === "periodic"
  compounding: Compounding;
  linked_account_id?: string; // UUIDv7 - destination for periodic payouts
}

/** POST /api/v1/accounts/:id/fixed-deposit/close */
export interface CloseFixedDepositRequest {
  destination_account_id?: string; // UUIDv7 - receives principal + interest
  interest_override?: number; // cents - override suggested accrued interest
  penalty?: number; // cents, default 0 (early-close fee)
}

export interface UpdateAccountRequest {
  name?: string;
  description?: string;
  is_active?: boolean;
  is_pinned?: boolean;
  broker?: string;
  account_number?: string;
  interest_rate?: number;
  due_date?: string;
  credit_limit?: number;
  // Fixed deposit mutable fields
  maturity_date?: string; // "YYYY-MM-DD"
  payout_type?: PayoutType;
  payout_frequency?: PayoutFrequency;
  compounding?: Compounding;
  linked_account_id?: string; // UUIDv7
}

// Transaction requests
export interface CreateTransactionRequest {
  account_id: string; // UUIDv7
  category_id?: string; // UUIDv7
  type: TransactionType;
  amount: number; // cents, > 0
  description?: string;
  date?: string; // ISO 8601
}

export interface CreateTransferRequest {
  from_account_id: string; // UUIDv7
  to_account_id: string; // UUIDv7
  amount: number; // cents, > 0
  description?: string;
  date?: string; // ISO 8601
  /** Cents credited to the destination, in its currency. Required when the accounts' currencies differ (else 400 FX_AMOUNT_REQUIRED), omit otherwise. */
  to_amount?: number;
}

export interface UpdateTransactionRequest {
  account_id?: string; // UUIDv7
  category_id?: string | null; // UUIDv7
  type?: TransactionType;
  amount?: number; // cents, > 0
  description?: string;
  date?: string; // ISO 8601
}

export interface TransactionFilters extends PaginationParams {
  from_date?: string;
  to_date?: string;
  type?: TransactionType;
  category_id?: string; // UUIDv7
  min_amount?: number;
  max_amount?: number;
}

export interface UserTransactionFilters extends TransactionFilters {
  account_id?: string; // UUIDv7
}

// Category requests
export interface CreateCategoryRequest {
  name: string;
  type: CategoryType;
  description?: string;
  icon?: string;
  color?: string; // hex color
  parent_id?: string; // UUIDv7
}

export interface UpdateCategoryRequest {
  name?: string;
  description?: string;
  icon?: string;
  color?: string;
  parent_id?: string; // UUIDv7
}

// Transaction rule responses (plan 018)
export interface RuleResponse {
  rule: TransactionRule;
}

export interface RulesResponse {
  rules: TransactionRule[];
}

// Rule request building blocks (mirror the backend service inputs)
export interface RuleConditionInput {
  field: RuleField;
  operator: RuleOperator;
  value_text?: string;
  amount_min?: number | null; // cents
  amount_max?: number | null; // cents
}

export interface RuleActionInput {
  action_type: RuleActionType;
  category_id?: string; // UUIDv7, for set_category
  value_text?: string;
}

export interface CreateRuleRequest {
  name: string;
  priority?: number;
  is_active?: boolean;
  conditions: RuleConditionInput[];
  actions: RuleActionInput[];
}

export interface UpdateRuleRequest {
  name?: string;
  priority?: number;
  is_active?: boolean;
  conditions?: RuleConditionInput[];
  actions?: RuleActionInput[];
}

export interface ReorderRulesRequest {
  rule_ids: string[];
}

export interface RulePreviewRequest {
  conditions: RuleConditionInput[];
}

// Preview / apply results (match backend RuleMatchPreview / ApplyRuleResult)
export interface RuleMatchPreview {
  count: number;
  sample: Transaction[];
}

export type RuleApplyScope = "uncategorized" | "all";

export interface ApplyRuleRequest {
  scope?: RuleApplyScope;
  overwrite?: boolean;
  dry_run?: boolean;
}

export interface ApplyRuleResult {
  count: number;
  applied: number;
  sample: Transaction[];
}

// Budget responses
export interface BudgetResponse {
  budget: Budget;
}

export interface BudgetProgressResponse {
  progress: BudgetProgress;
}

// Batch progress for all active budgets (single round-trip)
export interface BudgetsProgressResponse {
  progress: BudgetProgress[];
}

// Budget requests
export interface CreateBudgetRequest {
  category_id: string; // UUIDv7
  name: string;
  amount: number; // cents, > 0
  period: BudgetPeriod;
  /** ISO 4217; defaults to the user's base currency. */
  currency?: string;
}

export interface UpdateBudgetRequest {
  name?: string;
  amount?: number; // cents, > 0
  period?: BudgetPeriod;
  is_active?: boolean;
  /** ISO 4217 */
  currency?: string;
}

// Budget filters
export interface BudgetFilters extends PaginationParams {
  is_active?: boolean;
  period?: BudgetPeriod;
}

// Chart/analytics response types
export interface SpendingByCategoryItem {
  category_id: string | null; // UUIDv7
  category_name: string;
  category_color: string;
  category_icon: string;
  total: number; // cents, in the response's currency (base)
}

/** A native-currency amount left out of converted totals for lack of an FX rate. */
export interface UnconvertedAmount {
  currency: string;
  amount: number; // cents, in `currency`
}

export interface SpendingByCategory {
  items: SpendingByCategoryItem[];
  total_spent: number; // cents, in `currency`
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
  /** The user's base currency: the unit of every amount. */
  currency: string;
  /** Some spending had no exchange rate and was left out. */
  fx_missing: boolean;
  /** Native amounts left out of the totals (may be null/absent when none). */
  unconverted?: UnconvertedAmount[] | null;
}

export interface Cashflow {
  income: SpendingByCategoryItem[];
  expenses: SpendingByCategoryItem[];
  total_income: number; // cents
  total_expenses: number; // cents
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
  /** The user's base currency: the unit of every amount. */
  currency: string;
  fx_missing: boolean;
  /** Native amounts left out of the totals (may be null/absent when none). */
  unconverted?: UnconvertedAmount[] | null;
}

export interface MonthlySummaryItem {
  month: string; // "2026-02"
  income: number; // cents, base currency
  expenses: number; // cents, base currency
  /** The user's base currency. */
  currency: string;
  /** Some of the month's amounts had no rate and are excluded. */
  fx_missing: boolean;
}

export interface DailySpendingItem {
  date: string; // "2026-02-01"
  total: number; // cents, base currency
  /** The user's base currency. */
  currency: string;
  fx_missing: boolean;
}

export interface DailySummaryItem {
  date: string; // "2026-02-01"
  income: number; // cents, base currency
  expenses: number; // cents, base currency
  /** The user's base currency. */
  currency: string;
  fx_missing: boolean;
}

export interface TopExpenseItem {
  id: string;
  account_id: string;
  account_name: string;
  category_id: string | null; // UUIDv7
  category_name: string;
  category_color: string;
  category_icon: string;
  amount: number; // cents (full amount), in `currency`
  /** Cents the user bears, in `currency`; null when the expense is not shared. */
  personal_amount: number | null;
  description: string;
  date: string; // ISO 8601
  /** The account's currency: the unit of amount and personal_amount. */
  currency: string;
  /** Effective amount (personal share, else amount) in the base currency at the expense's date; 0 when fx_missing. */
  amount_base: number;
  /** No rate existed, so amount_base could not be computed. */
  fx_missing: boolean;
}

export interface TopExpenses {
  items: TopExpenseItem[];
  /** Expenses with a non-zero effective amount in the range (ignores limit). */
  total_count: number;
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
  /** The user's base currency (the unit of each item's amount_base). */
  currency: string;
  /** Some expenses had no rate; they rank after every converted expense. */
  fx_missing: boolean;
}

// Investment response wrappers
export interface InvestmentResponse {
  investment: Investment;
}

export interface InvestmentTransactionResponse {
  transaction: InvestmentTransaction;
}

export interface SecurityResponse {
  security: Security;
}

export interface PortfolioResponse {
  portfolio: PortfolioSummary;
}

// Investment requests
export interface AddInvestmentRequest {
  account_id: string; // UUIDv7
  security_id: string; // UUIDv7
  quantity: number; // float, > 0
  purchase_price: number; // cents, > 0, security currency
  wallet_address?: string;
  date?: string; // ISO 8601, defaults to now
  fee?: number; // cents, >= 0, defaults to 0 (security currency)
  notes?: string; // max 500, defaults to "Initial purchase"
  /** Optional decimal string: security currency -> base rate of the trade. */
  fx_rate?: string;
}

export interface RecordBuyRequest {
  date: string; // ISO 8601
  quantity: number; // float, > 0
  price_per_unit: number; // cents, > 0
  fee?: number; // cents, >= 0
  notes?: string;
  funding_account_id?: string; // optional cash account to deduct from
  /** Cents debited from the funding account, in ITS currency; required when it differs from the security's currency. */
  funding_amount?: number;
  /** Optional decimal string: security currency -> base rate of the trade. */
  fx_rate?: string;
}

export interface RecordSellRequest {
  date: string; // ISO 8601
  quantity: number; // float, > 0
  price_per_unit: number; // cents, > 0
  fee?: number; // cents, >= 0
  notes?: string;
  deposit_account_id?: string; // optional cash account to credit with proceeds
  /** Cents credited to the deposit account, in ITS currency; required when it differs from the security's currency. */
  deposit_amount?: number;
  /** Optional decimal string: security currency -> base rate of the trade. */
  fx_rate?: string;
}

export interface RecordDividendRequest {
  date: string; // ISO 8601
  amount: number; // cents, > 0, gross, security currency
  dividend_type?: string;
  notes?: string;
  /** Withholding tax plus fees, cents in the security currency (<= amount). */
  fee?: number;
  /** Optional cash account credited with the NET dividend (amount - fee). */
  deposit_account_id?: string;
  /** Net cents credited in the deposit account's currency; required when it differs from the security's currency. */
  deposit_amount?: number;
  /** Optional decimal string: security currency -> base rate of the trade. */
  fx_rate?: string;
}

export interface RecordSplitRequest {
  date: string; // ISO 8601
  split_ratio: number; // float, > 0
  notes?: string;
}

// Security filters
export interface SecurityFilters extends PaginationParams {
  search?: string;
}

// Portfolio snapshot filters
export interface PortfolioSnapshotFilters {
  from_date: string;
  to_date: string;
  group_by?: "day" | "hour";
  page?: number;
  page_size?: number;
}

// Response shape for grouped (downsampled) snapshot queries
export interface GroupedSnapshotResponse<T> {
  data: T[];
}

// OAuth login/consent bridge (Hydra) — see plans/015-mcp-oauth-hydra Phase 1.
export interface OAuthLoginRequest {
  login_challenge: string;
  email: string;
  password: string;
}

// Both /oauth/login and /oauth/consent/accept return a Hydra redirect target.
export interface OAuthRedirectResponse {
  redirect_to: string;
}

export interface OAuthConsentClient {
  client_id: string;
  client_name: string;
}

// GET /oauth/consent returns either redirect_to (remembered client, auto-accepted
// for scopes the user already reviewed) or the details the consent screen needs
// to render, including the scopes the user approved last time (plan 022).
export interface OAuthConsentDetails {
  redirect_to?: string;
  client?: OAuthConsentClient;
  requested_scopes?: string[];
  previously_approved?: string[];
  redirect_uris?: string[];
}

// granted_scopes lists the API scopes the user ticked; protocol scopes are
// granted automatically.
export interface OAuthConsentAcceptRequest {
  consent_challenge: string;
  remember_client: boolean;
  granted_scopes: string[];
}

// An OAuth client (e.g. a Claude connector) the user has granted access to.
// legacy marks a connection made before per-scope grants: it can read but
// never write, and its connection date is unknown.
export interface ConnectedApp {
  client_id: string;
  client_name: string;
  scopes: string[];
  auto_accept: boolean;
  legacy?: boolean;
  created_at?: string;
  // Updated only when the app makes a change (an MCP write), not on reads.
  last_used_at?: string;
}

export interface ConnectedAppsResponse {
  clients: ConnectedApp[];
}

// ---- Receivables / shared expenses (plan 021) ----

export interface ShareCounterpartyInput {
  name: string;
  /** cents; omit for `equal` and `full` modes */
  amount?: number;
}

// POST /transactions/shared
export interface CreateSharedExpenseRequest {
  account_id: string; // UUIDv7
  category_id?: string | null; // UUIDv7
  amount: number; // cents, > 0
  description?: string; // max 500
  date?: string; // ISO 8601
  kind: ReceivableKind;
  mode: ShareMode;
  counterparties: ShareCounterpartyInput[]; // 1..20
}

export interface ReceivableShareInput {
  id?: string; // UUIDv7; omit to insert
  name: string;
  amount: number; // cents
}

// PUT /transactions/:id/receivables (diffed, not replaced)
export interface UpdateReceivablesRequest {
  kind?: ReceivableKind;
  receivables: ReceivableShareInput[];
}

export interface ReceivableFilters extends PaginationParams {
  /** A status, or "open" (pending | partially_paid). */
  status?: ReceivableStatus | "open";
  kind?: ReceivableKind;
  /** A counterparty key (lowercased name). */
  counterparty?: string;
}

export interface ReceivableResponse {
  receivable: Receivable;
}

export interface SettleReceivableResponse {
  receivable: Receivable;
  settlement: ReceivableSettlement;
}

export interface SettleReceivableRequest {
  account_id: string; // UUIDv7
  amount: number; // cents
  date?: string; // ISO 8601
  description?: string;
  write_off_remainder?: boolean;
  /** Cents credited to the account, in the account's currency; required when it differs from the receivable's currency. */
  account_amount?: number;
}

export interface WriteOffReceivableRequest {
  /** cents; defaults to the whole outstanding balance */
  amount?: number;
}

export interface ReceivableKindSummary {
  kind: ReceivableKind;
  outstanding: number; // cents
  open_count: number;
}

/** Open outstanding in one receivable currency, unconverted. */
export interface ReceivableCurrencyTotal {
  currency: string;
  outstanding: number; // cents, in `currency`
}

export interface ReceivableCounterpartySummary {
  counterparty_key: string;
  counterparty_name: string;
  outstanding: number; // cents
  open_count: number;
  oldest_date: string; // ISO 8601
}

export interface ReceivablesSummary {
  total_outstanding: number; // cents, in `currency` (the user's base)
  /** Currency of the converted totals (the user's base). */
  currency: string;
  /** Some outstanding amounts had no rate and are left out of the totals. */
  fx_missing: boolean;
  /** Native outstanding per receivable currency (may be null/absent when none). */
  by_currency?: ReceivableCurrencyTotal[] | null;
  by_kind: ReceivableKindSummary[];
  by_counterparty: ReceivableCounterpartySummary[];
}

export interface Counterparty {
  key: string;
  name: string;
}

export interface CounterpartiesResponse {
  counterparties: Counterparty[];
}

export interface RenameCounterpartyRequest {
  from_key: string;
  to_name: string;
}

export interface RenameCounterpartyResponse {
  updated: number;
}

/**
 * GET /api/v1/system/status. Each field is an RFC 3339 time, or null when it
 * has never happened (or is not available). `latest_snapshot_at` is the
 * caller's own latest portfolio snapshot.
 */
export interface SystemStatus {
  latest_price_at: string | null;
  latest_fx_rate_at: string | null;
  latest_snapshot_at: string | null;
  last_backup_at: string | null;
}
