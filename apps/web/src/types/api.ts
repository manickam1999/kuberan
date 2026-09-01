import type {
  Account,
  Attachment,
  Budget,
  BudgetPeriod,
  BudgetProgress,
  Category,
  Investment,
  InvestmentTransaction,
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
}

export interface RefreshRequest {
  refresh_token: string;
}

// Auth response (login, register, refresh all return this)
export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: User;
}

// Profile response
export interface ProfileResponse {
  user: User;
}

// Request to update profile settings (PATCH /profile)
export interface UpdateProfileSettingsRequest {
  hide_balances: boolean;
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
  interest_rate?: number;
  due_date?: string;
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
}

export interface UpdateBudgetRequest {
  name?: string;
  amount?: number; // cents, > 0
  period?: BudgetPeriod;
  is_active?: boolean;
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
  total: number; // cents
}

export interface SpendingByCategory {
  items: SpendingByCategoryItem[];
  total_spent: number; // cents
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
}

export interface Cashflow {
  income: SpendingByCategoryItem[];
  expenses: SpendingByCategoryItem[];
  total_income: number; // cents
  total_expenses: number; // cents
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
}

export interface MonthlySummaryItem {
  month: string; // "2026-02"
  income: number; // cents
  expenses: number; // cents
}

export interface DailySpendingItem {
  date: string; // "2026-02-01"
  total: number; // cents
}

export interface DailySummaryItem {
  date: string; // "2026-02-01"
  income: number; // cents
  expenses: number; // cents
}

export interface TopExpenseItem {
  id: string;
  account_id: string;
  account_name: string;
  category_id: string | null; // UUIDv7
  category_name: string;
  category_color: string;
  category_icon: string;
  amount: number; // cents
  description: string;
  date: string; // ISO 8601
}

export interface TopExpenses {
  items: TopExpenseItem[];
  from_date: string; // ISO 8601
  to_date: string; // ISO 8601
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
  purchase_price: number; // cents, > 0
  wallet_address?: string;
  date?: string; // ISO 8601, defaults to now
  fee?: number; // cents, >= 0, defaults to 0
  notes?: string; // max 500, defaults to "Initial purchase"
}

export interface RecordBuyRequest {
  date: string; // ISO 8601
  quantity: number; // float, > 0
  price_per_unit: number; // cents, > 0
  fee?: number; // cents, >= 0
  notes?: string;
  funding_account_id?: string; // optional cash account to deduct from
}

export interface RecordSellRequest {
  date: string; // ISO 8601
  quantity: number; // float, > 0
  price_per_unit: number; // cents, > 0
  fee?: number; // cents, >= 0
  notes?: string;
  deposit_account_id?: string; // optional cash account to credit with proceeds
}

export interface RecordDividendRequest {
  date: string; // ISO 8601
  amount: number; // cents, > 0
  dividend_type?: string;
  notes?: string;
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

// GET /oauth/consent returns either redirect_to (trusted client, auto-accepted)
// or the details an unknown client's consent screen needs to render.
export interface OAuthConsentDetails {
  redirect_to?: string;
  client?: OAuthConsentClient;
  requested_scopes?: string[];
  redirect_uris?: string[];
}

export interface OAuthConsentAcceptRequest {
  consent_challenge: string;
  remember_client: boolean;
}
