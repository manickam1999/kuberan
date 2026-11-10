import type { SpendingByCategoryItem, TypicalMonth } from "@/types/api";

/** True when the user has at least one complete month of history to compare with. */
export function hasTypical(t: TypicalMonth | undefined): t is TypicalMonth {
  return !!t && t.months_used > 0 && t.daily_cumulative.length > 0;
}

/**
 * Typical cumulative spending after `day` (1-based day of the month). Days
 * past the end of the curve use its last point; an empty curve gives 0.
 */
export function typicalAtDay(curve: readonly number[], day: number): number {
  if (curve.length === 0) return 0;
  const i = Math.min(Math.max(Math.floor(day), 1), curve.length) - 1;
  return curve[i];
}

/**
 * How far `actual` is from `typical`, as a whole percent (negative = below).
 * Null when there is nothing to compare with.
 */
export function vsTypicalPercent(
  actual: number,
  typical: number,
): number | null {
  if (typical <= 0) return null;
  return Math.round(((actual - typical) / typical) * 100);
}

/** "−18% vs typical", "+12% vs typical" or "On par with typical". */
export function vsTypicalLabel(pct: number): string {
  if (pct === 0) return "On par with typical";
  return `${pct < 0 ? "−" : "+"}${Math.abs(pct)}% vs typical`;
}

/** Spending below typical reads as good news, above as bad. */
export function vsTypicalTone(pct: number | null): "pos" | "neg" | "default" {
  if (pct === null || pct === 0) return "default";
  return pct < 0 ? "pos" : "neg";
}

/** Parses the year and 0-based month out of a local "YYYY-MM-DD" date. */
function yearMonth(date: string): { year: number; month: number } {
  const [y, m] = date.split("-").map(Number);
  return { year: y, month: m - 1 };
}

/**
 * What a typical month's spending comes to by `today` for the month that
 * starts at `monthStart` ("YYYY-MM-DD"): the full average for a past month,
 * the typical pace up to today for the current month, and null for a future
 * month or when there is no history.
 */
export function typicalSpendFor(
  typical: TypicalMonth | undefined,
  monthStart: string,
  today: Date,
): number | null {
  if (!hasTypical(typical)) return null;
  const { year, month } = yearMonth(monthStart);
  const here = today.getFullYear() * 12 + today.getMonth();
  const there = year * 12 + month;
  if (there > here) return null;
  if (there < here) return typical.avg_spending;
  return typicalAtDay(typical.daily_cumulative, today.getDate());
}

/**
 * The factor that turns a typical month's per-category averages into what is
 * typical by today (1 for a past month, the pace share for the current one).
 * Null when no comparison makes sense.
 */
export function typicalScale(
  typical: TypicalMonth | undefined,
  monthStart: string,
  today: Date,
): number | null {
  const spend = typicalSpendFor(typical, monthStart, today);
  if (spend === null || !hasTypical(typical) || typical.avg_spending <= 0) {
    return spend === null ? null : 1;
  }
  return spend / typical.avg_spending;
}

/** Stable key of a category row; uncategorised spending has a null id. */
export function categoryKey(item: Pick<SpendingByCategoryItem, "category_id">) {
  return item.category_id ?? "uncategorized";
}

export interface CategoryTypical {
  /** Typical amount for the period, in cents. */
  typical: number;
  /** Percent versus typical; null for a category with no history. */
  pct: number | null;
}

/**
 * Compares each category's spending with its typical amount, scaled to the
 * period. Categories the user has no history for map to a typical of 0.
 */
export function categoryComparison(
  items: readonly SpendingByCategoryItem[],
  typical: TypicalMonth,
  scale: number,
): Map<string, CategoryTypical> {
  const avg = new Map(
    typical.by_category.map((c) => [categoryKey(c), c.total] as const),
  );
  return new Map(
    items.map((item) => {
      const t = Math.round((avg.get(categoryKey(item)) ?? 0) * scale);
      return [
        categoryKey(item),
        { typical: t, pct: vsTypicalPercent(item.total, t) },
      ] as const;
    }),
  );
}

/**
 * The running spend of a month against the typical curve for days 1 to
 * `throughDay`. `actual` holds cumulative cents per day, day 1 first.
 */
export function paceSeries(
  actual: readonly number[],
  curve: readonly number[],
  throughDay: number,
): { actual: number[]; typical: number[] } {
  const n = Math.max(0, Math.min(throughDay, actual.length));
  return {
    actual: actual.slice(0, n),
    typical: Array.from({ length: n }, (_, i) => typicalAtDay(curve, i + 1)),
  };
}
