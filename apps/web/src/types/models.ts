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

// Budget periods
export type BudgetPeriod = "monthly" | "yearly";

export interface Budget extends BaseModel {
  user_id: string; // UUIDv7
  category_id: string; // UUIDv7
  name: string;
  amount: number; // cents
  period: BudgetPeriod;
  start_date: string; // ISO 8601
  end_date?: string | null; // ISO 8601
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
  cost_basis: number; // cents
  realized_gain_loss: number; // cents — accumulated realized P&L from sells
  current_price: number; // cents per unit, populated at query time from security_prices
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
