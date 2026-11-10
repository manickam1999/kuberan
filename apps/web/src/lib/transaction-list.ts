import { toLocalDateString } from "@/lib/format";
import type { Transaction, TransactionType } from "@/types/models";
import type { UserTransactionFilters } from "@/types/api";

/** The date part of a transaction's ISO date (dates are calendar days). */
export function dayKey(iso: string): string {
  return iso.split("T")[0];
}

export interface DayGroup<T> {
  /** YYYY-MM-DD. */
  key: string;
  items: T[];
}

/** Group rows by calendar day, keeping the incoming (date DESC) order. */
export function groupByDay<T extends { date: string }>(rows: T[]): DayGroup<T>[] {
  const groups: DayGroup<T>[] = [];
  const byKey = new Map<string, DayGroup<T>>();
  for (const row of rows) {
    const key = dayKey(row.date);
    let group = byKey.get(key);
    if (!group) {
      group = { key, items: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.items.push(row);
  }
  return groups;
}

/**
 * Net cash flow of a day per currency. Amounts in different currencies are
 * never added together. Transfers, investments and opening balances are not
 * cash flow; repayments are inflows.
 */
export function dayNetByCurrency(
  rows: Transaction[],
  currencyOf: (tx: Transaction) => string
): [string, number][] {
  const nets = new Map<string, number>();
  for (const t of rows) {
    const delta =
      t.type === "income" || t.type === "reimbursement"
        ? t.amount
        : t.type === "expense"
          ? -t.amount
          : 0;
    if (delta === 0) continue;
    const ccy = currencyOf(t);
    nets.set(ccy, (nets.get(ccy) ?? 0) + delta);
  }
  return Array.from(nets).filter(([, n]) => n !== 0);
}

/** Total of the rows when they share one currency, else null. */
export function sameCurrencySum(
  rows: Transaction[],
  currencyOf: (tx: Transaction) => string
): { amount: number; currency: string } | null {
  if (rows.length === 0) return null;
  const currency = currencyOf(rows[0]);
  let amount = 0;
  for (const t of rows) {
    if (currencyOf(t) !== currency) return null;
    if (t.type === "income" || t.type === "reimbursement") amount += t.amount;
    else if (t.type === "expense") amount -= t.amount;
    else return null;
  }
  return { amount, currency };
}

export type PeriodKey = "all" | "this-month" | "last-month" | "last-90" | "custom";

export const PERIOD_LABELS: Record<PeriodKey, string> = {
  all: "All time",
  "this-month": "This month",
  "last-month": "Last month",
  "last-90": "Last 90 days",
  custom: "Custom range",
};

/** Local YYYY-MM-DD bounds of a period; open-ended for "all". */
export function periodRange(
  period: PeriodKey,
  now: Date,
  custom: { from?: string; to?: string } = {}
): { from?: string; to?: string } {
  const y = now.getFullYear();
  const m = now.getMonth();
  switch (period) {
    case "this-month":
      return {
        from: toLocalDateString(new Date(y, m, 1)),
        to: toLocalDateString(new Date(y, m + 1, 0)),
      };
    case "last-month":
      return {
        from: toLocalDateString(new Date(y, m - 1, 1)),
        to: toLocalDateString(new Date(y, m, 0)),
      };
    case "last-90":
      return {
        from: toLocalDateString(new Date(y, m, now.getDate() - 89)),
        to: toLocalDateString(now),
      };
    case "custom":
      return { from: custom.from || undefined, to: custom.to || undefined };
    default:
      return {};
  }
}

/** Everything the Transactions page keeps in the URL. */
export interface TxQueryState {
  q: string;
  period: PeriodKey;
  from: string;
  to: string;
  account: string;
  category: string;
  type: string;
  /** Major units as typed ("12.50"); empty when unset. */
  min: string;
  max: string;
  shared: boolean;
  receipt: boolean;
  page: number;
}

export const EMPTY_TX_QUERY: TxQueryState = {
  q: "",
  period: "all",
  from: "",
  to: "",
  account: "",
  category: "",
  type: "",
  min: "",
  max: "",
  shared: false,
  receipt: false,
  page: 1,
};

const PERIODS: readonly PeriodKey[] = [
  "all",
  "this-month",
  "last-month",
  "last-90",
  "custom",
];

const TYPES: readonly TransactionType[] = [
  "income",
  "expense",
  "transfer",
  "reimbursement",
  "opening_balance",
  "investment",
];

/** Parse the page's search params; unknown or malformed values fall back. */
export function parseTxQuery(params: URLSearchParams): TxQueryState {
  const period = params.get("period") as PeriodKey | null;
  const type = params.get("type") ?? "";
  const page = Number.parseInt(params.get("page") ?? "1", 10);
  const amount = (key: string) => {
    const v = params.get(key) ?? "";
    return /^\d+(\.\d{1,2})?$/.test(v) ? v : "";
  };
  const date = (key: string) => {
    const v = params.get(key) ?? "";
    return /^\d{4}-\d{2}-\d{2}$/.test(v) ? v : "";
  };
  const from = date("from");
  const to = date("to");
  // Links from other screens (Insights) send from/to without a period, and
  // the API's names category_id/account_id; accept both forms.
  const parsedPeriod: PeriodKey =
    period && PERIODS.includes(period)
      ? period
      : from || to
        ? "custom"
        : "all";
  return {
    q: (params.get("q") ?? "").slice(0, 100),
    period: parsedPeriod,
    from,
    to,
    account: params.get("account") ?? params.get("account_id") ?? "",
    category: params.get("category") ?? params.get("category_id") ?? "",
    type: (TYPES as readonly string[]).includes(type) ? type : "",
    min: amount("min"),
    max: amount("max"),
    shared: params.get("shared") === "1",
    receipt: params.get("receipt") === "1",
    page: Number.isFinite(page) && page > 1 ? page : 1,
  };
}

/** Serialise only the non-default parts of the state. */
export function serializeTxQuery(state: TxQueryState): string {
  const p = new URLSearchParams();
  if (state.q) p.set("q", state.q);
  if (state.period !== "all") p.set("period", state.period);
  if (state.period === "custom") {
    if (state.from) p.set("from", state.from);
    if (state.to) p.set("to", state.to);
  }
  if (state.account) p.set("account", state.account);
  if (state.category) p.set("category", state.category);
  if (state.type) p.set("type", state.type);
  if (state.min) p.set("min", state.min);
  if (state.max) p.set("max", state.max);
  if (state.shared) p.set("shared", "1");
  if (state.receipt) p.set("receipt", "1");
  if (state.page > 1) p.set("page", String(state.page));
  return p.toString();
}

/** "12.50" to 1250 cents. */
export function majorToCents(value: string): number | undefined {
  if (!value) return undefined;
  const n = Number.parseFloat(value);
  return Number.isFinite(n) ? Math.round(n * 100) : undefined;
}

/** Number of active filters (the search box and paging do not count). */
export function activeFilterCount(state: TxQueryState): number {
  return [
    state.period !== "all",
    !!state.account,
    !!state.category,
    !!state.type,
    !!(state.min || state.max),
    state.shared,
    state.receipt,
  ].filter(Boolean).length;
}

/** The API filters for a page state. */
export function toFilters(
  state: TxQueryState,
  pageSize: number,
  now: Date = new Date()
): UserTransactionFilters {
  const range = periodRange(state.period, now, {
    from: state.from,
    to: state.to,
  });
  return {
    page: state.page,
    page_size: pageSize,
    q: state.q.trim() || undefined,
    from_date: range.from,
    to_date: range.to,
    account_id: state.account || undefined,
    category_id: state.category || undefined,
    type: (state.type as TransactionType) || undefined,
    min_amount: majorToCents(state.min),
    max_amount: majorToCents(state.max),
    shared: state.shared ? true : undefined,
    has_attachment: state.receipt ? true : undefined,
  };
}

/**
 * The most recently used category ids (distinct, newest first) among
 * transactions of one type, for the quick chips in the add dialog.
 */
export function recentCategoryIds(
  rows: Pick<Transaction, "type" | "category_id" | "date">[],
  type: TransactionType,
  limit: number
): string[] {
  const seen = new Set<string>();
  const sorted = [...rows].sort((a, b) => (a.date < b.date ? 1 : -1));
  for (const row of sorted) {
    if (row.type !== type || !row.category_id) continue;
    seen.add(row.category_id);
    if (seen.size >= limit) break;
  }
  return Array.from(seen);
}
