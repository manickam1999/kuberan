import type {
  Account,
  Budget,
  BudgetPeriod,
  BudgetProgress,
  Category,
  Investment,
  InvestmentTransaction,
  PortfolioSummary,
  Security,
  Transaction,
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

// Budget responses
export interface BudgetResponse {
  budget: Budget;
}

export interface BudgetProgressResponse {
  progress: BudgetProgress;
}

// Budget requests
export interface CreateBudgetRequest {
  category_id: string; // UUIDv7
  name: string;
  amount: number; // cents, > 0
  period: BudgetPeriod;
  start_date: string; // ISO 8601
  end_date?: string; // ISO 8601
}

export interface UpdateBudgetRequest {
  name?: string;
  amount?: number; // cents, > 0
  period?: BudgetPeriod;
  end_date?: string; // ISO 8601
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

export interface MonthlySummaryItem {
  month: string; // "2026-02"
  income: number; // cents
  expenses: number; // cents
}

export interface DailySpendingItem {
  date: string; // "2026-02-01"
  total: number; // cents
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
}

export interface RecordSellRequest {
  date: string; // ISO 8601
  quantity: number; // float, > 0
  price_per_unit: number; // cents, > 0
  fee?: number; // cents, >= 0
  notes?: string;
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
