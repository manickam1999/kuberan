import type { Cashflow, SpendingByCategoryItem } from "@/types/api";

/**
 * Link to Transactions filtered the way an Insights mark describes it.
 * Query names: `category_id`, `type` (`expense` | `income`), `from` and `to`
 * (local YYYY-MM-DD, both inclusive).
 */
export function transactionsHref(filter: {
  from?: string;
  to?: string;
  type?: "expense" | "income";
  categoryId?: string | null;
}): string {
  const params = new URLSearchParams();
  if (filter.categoryId) params.set("category_id", filter.categoryId);
  if (filter.type) params.set("type", filter.type);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  const qs = params.toString();
  return qs ? `/transactions?${qs}` : "/transactions";
}

/** First and last local date (YYYY-MM-DD) of a "YYYY-MM" month key. */
export function monthBounds(month: string): { from: string; to: string } {
  const [y, m] = month.split("-").map(Number);
  const pad = (n: number) => String(n).padStart(2, "0");
  const last = new Date(y, m, 0).getDate();
  return { from: `${y}-${pad(m)}-01`, to: `${y}-${pad(m)}-${pad(last)}` };
}

export interface RankedFlowRow {
  key: string;
  label: string;
  amount: number;
  kind: "expense" | "other" | "saved" | "deficit";
  /** Present for a single category; the row links to its transactions. */
  categoryId?: string | null;
  color?: string;
}

/**
 * The mobile stand-in for the sankey: the biggest spending categories, the
 * rest folded into "Everything else", then what was saved (or drawn from
 * savings when spending outran income).
 */
export function rankedFlow(data: Cashflow, topN = 3): RankedFlowRow[] {
  const expenses: SpendingByCategoryItem[] = data.expenses
    .filter((e) => e.total > 0)
    .sort((a, b) => b.total - a.total);
  const rows: RankedFlowRow[] = expenses.slice(0, topN).map((e) => ({
    key: e.category_id ?? e.category_name,
    label: e.category_name,
    amount: e.total,
    kind: "expense",
    categoryId: e.category_id,
    color: e.category_color,
  }));
  const rest = expenses.slice(topN).reduce((sum, e) => sum + e.total, 0);
  if (rest > 0) {
    rows.push({
      key: "rest",
      label: "Everything else",
      amount: rest,
      kind: "other",
    });
  }
  const surplus = data.total_income - data.total_expenses;
  if (surplus > 0) {
    rows.push({ key: "saved", label: "Saved", amount: surplus, kind: "saved" });
  } else if (surplus < 0) {
    rows.push({
      key: "deficit",
      label: "From savings",
      amount: -surplus,
      kind: "deficit",
    });
  }
  return rows;
}

/** Share of income kept, as a whole-number percent; null without income. */
export function savingsRate(income: number, expenses: number): number | null {
  if (income <= 0) return null;
  return Math.round(((income - expenses) / income) * 100);
}

/**
 * Heatmap intensity for a day's spending (cents): 0 for none, else 0.15 to 1
 * on a log scale so one big day doesn't flatten the rest.
 */
export function heatIntensity(cents: number): number {
  if (cents <= 0) return 0;
  const units = cents / 100;
  return Math.min(
    1,
    Math.max(0.15, 0.15 + Math.log10(Math.max(units, 1)) / 3.6)
  );
}
