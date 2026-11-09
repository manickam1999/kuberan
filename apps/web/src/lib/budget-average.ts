import { toLocalDateString } from "@/lib/format";

/**
 * The six complete calendar months before the month of `now`, as local
 * YYYY-MM-DD bounds (the current month is partial, so it would drag an
 * average down).
 */
export function lastSixMonthsRange(now: Date = new Date()): {
  from: string;
  to: string;
} {
  const first = new Date(now.getFullYear(), now.getMonth() - 6, 1);
  const last = new Date(now.getFullYear(), now.getMonth(), 0);
  return { from: toLocalDateString(first), to: toLocalDateString(last) };
}

/** Monthly average (cents) of a six-month total, rounded to the nearest cent. */
export function sixMonthAverage(total: number): number {
  return Math.round(total / 6);
}
