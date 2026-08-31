// Domain models matching backend API responses exactly.
// All monetary values are int64 cents (e.g., $10.50 = 1050).
// All IDs are UUIDv7 strings.

// Base fields present on all entities
export interface BaseModel {
  id: string; // UUIDv7
  created_at: string; // ISO 8601
  updated_at: string; // ISO 8601
  deleted_at?: string | null; // ISO 8601, only present if soft-deleted
}

// User (from GET /profile, auth responses)
export interface User {
  id: string; // UUIDv7
  email: string;
  first_name: string;
  last_name: string;
  is_active?: boolean;
  last_login_at?: string | null; // ISO 8601
  hide_balances: boolean;
}

// Account types
export type AccountType = "cash" | "investment" | "debt" | "credit_card";

export interface Account extends BaseModel {
  user_id: string; // UUIDv7
  name: string;
  type: AccountType;
  description: string;
  balance: number; // cents
  currency: string; // ISO 4217
  is_active: boolean;
  is_pinned: boolean;
  broker?: string; // investment accounts
  account_number?: string; // investment accounts
  interest_rate?: number; // debt/credit_card accounts (float)
  due_date?: string; // debt/credit_card accounts, ISO 8601
  credit_limit?: number; // credit_card accounts (cents)
}

// Transaction types
export type TransactionType = "income" | "expense" | "transfer" | "investment";

export interface Transaction extends BaseModel {
  user_id: string; // UUIDv7
  account_id: string; // UUIDv7
  category_id?: string | null; // UUIDv7
  type: TransactionType;
  amount: number; // cents, always positive
  description: string;
  date: string; // ISO 8601
  to_account_id?: string | null; // UUIDv7, for transfers
  account?: Account; // preloaded relation
  to_account?: Account | null; // preloaded relation for transfers
  category?: Category | null; // preloaded relation
  attachments?: Attachment[]; // preloaded on detail queries only (not list)
  attachments_count?: number; // populated on list queries for the paperclip indicator
}

// Receipt attachment (image/PDF) on a transaction. Bytes live in the blob
// store (MinIO/S3); this is metadata only. The opaque storage_key is never
// exposed. See plans/017-transaction-receipts.
export interface Attachment extends BaseModel {
  user_id: string; // UUIDv7
  transaction_id: string; // UUIDv7
  file_name: string;
  content_type: string; // e.g. image/jpeg, application/pdf
  byte_size: number; // bytes
}

// Category types
export type CategoryType = "income" | "expense";

export interface Category extends BaseModel {
  user_id: string; // UUIDv7
  name: string;
  type: CategoryType;
  description: string;
  icon: string;
  color: string; // hex (#RGB or #RRGGBB)
  parent_id?: string | null; // UUIDv7
  parent?: Category | null;
  children?: Category[];
}

// Transaction rules (auto-categorization). See plans/018-transaction-rules-engine.
export type RuleField = "description" | "amount" | "account_id" | "type";
export type RuleOperator =
  | "contains"
  | "not_contains"
  | "equals"
  | "starts_with"
  | "ends_with"
  | "gt"
  | "lt"
  | "between";
export type RuleActionType = "set_category";

export interface RuleCondition {
  id?: string; // UUIDv7 (present on read)
  field: RuleField;
  operator: RuleOperator;
  value_text?: string | null; // description text / account UUID / type value
  amount_min?: number | null; // cents (gt, between)
  amount_max?: number | null; // cents (lt, between)
}

export interface RuleAction {
  id?: string; // UUIDv7 (present on read)
  action_type: RuleActionType;
  category_id?: string | null; // UUIDv7, for set_category
  value_text?: string; // reserved for future actions
  category?: Category | null; // preloaded on read
}

// A rule = AND-ed conditions -> actions. Rules are OR-ed and evaluated
// first-match-wins by (priority ASC, created_at ASC).
export interface TransactionRule extends BaseModel {
  user_id: string; // UUIDv7
  name: string;
  priority: number; // lower = evaluated first
  is_active: boolean;
  conditions: RuleCondition[];
  actions: RuleAction[];
}

// Budget periods
export type BudgetPeriod = "monthly" | "yearly";

export interface Budget extends BaseModel {
  user_id: string; // UUIDv7
  category_id: string; // UUIDv7
  name: string;
  amount: number; // cents
  period: BudgetPeriod;
  is_active: boolean;
  category?: Category; // preloaded relation
}

export interface BudgetProgress {
  budget_id: string; // UUIDv7
  budgeted: number; // cents
  spent: number; // cents
  remaining: number; // cents
  percentage: number; // float, (spent/budgeted)*100
}

// Asset types
export type AssetType = "stock" | "etf" | "bond" | "crypto" | "reit" | "commodity";

// Security — shared entity for financial instruments
export interface Security extends BaseModel {
  symbol: string;
  name: string;
  asset_type: AssetType;
  currency: string; // ISO 4217
  exchange?: string;
  maturity_date?: string | null; // bonds, ISO 8601
  yield_to_maturity?: number; // bonds, float
  coupon_rate?: number; // bonds, float
  network?: string; // crypto
  property_type?: string; // REITs
}

// Security price (time-series, no soft deletes)
export interface SecurityPrice {
  id: string; // UUIDv7
  security_id: string; // UUIDv7
  price: number; // cents
  recorded_at: string; // ISO 8601
  security?: Security;
}

// Portfolio snapshot (time-series, no soft deletes)
export interface PortfolioSnapshot {
  id: string; // UUIDv7
  user_id: string; // UUIDv7
  recorded_at: string; // ISO 8601
  total_net_worth: number; // cents
  cash_balance: number; // cents
  investment_value: number; // cents
  debt_balance: number; // cents
}

export interface Investment extends BaseModel {
  account_id: string; // UUIDv7
  security_id: string; // UUIDv7
  quantity: number; // float
  cost_basis: number; // cents — remaining cost basis of open position; reduced proportionally on sells (0 for fully closed positions)
  realized_gain_loss: number; // cents — accumulated realized P&L from sells
  current_price: number; // cents per unit, populated at query time from security_prices
  total_invested: number; // cents — sum of buy transaction amounts, populated at query time (use for closed-position return %)
  wallet_address?: string; // crypto
  security: Security; // preloaded relation
  account?: Account; // preloaded relation
}

// Investment transaction types
export type InvestmentTransactionType =
  | "buy"
  | "sell"
  | "dividend"
  | "split"
  | "transfer";

export interface InvestmentTransaction extends BaseModel {
  investment_id: string; // UUIDv7
  type: InvestmentTransactionType;
  date: string; // ISO 8601
  quantity: number; // float
  price_per_unit: number; // cents
  total_amount: number; // cents
  fee: number; // cents
  notes: string;
  realized_gain_loss: number; // cents — realized P&L for this specific sell
  split_ratio?: number; // float, for splits
  dividend_type?: string; // for dividends
  investment?: Investment; // preloaded relation
}

export interface PortfolioSummary {
  total_value: number; // cents
  total_cost_basis: number; // cents
  total_gain_loss: number; // cents
  gain_loss_pct: number; // float percentage
  total_realized_gain_loss: number; // cents
  holdings_by_type: Record<AssetType, { value: number; count: number }>;
}
