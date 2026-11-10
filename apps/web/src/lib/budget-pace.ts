import type { BudgetPeriod } from "@/types/models";

export type BudgetStatus = "on_track" | "close" | "over";

/** At or above this share of the budget, a budget reads "Close". */
export const CLOSE_THRESHOLD = 80;

/**
 * How far through the budget's period `now` is, from 0 to 1, in local time.
 * Monthly budgets run over the calendar month, yearly ones over the calendar
 * year. Day granularity: the first day is 1/N elapsed, the last day is 1.
 */
export function periodElapsedFraction(
  period: BudgetPeriod,
  now: Date = new Date()
): number {
  const year = now.getFullYear();
  if (period === "monthly") {
    const days = new Date(year, now.getMonth() + 1, 0).getDate();
    return now.getDate() / days;
  }
  const start = Date.UTC(year, 0, 1);
  const today = Date.UTC(year, now.getMonth(), now.getDate());
  const dayOfYear = Math.round((today - start) / 86_400_000) + 1;
  const days = (Date.UTC(year + 1, 0, 1) - start) / 86_400_000;
  return dayOfYear / days;
}

/** Whole days left in the period after today (0 on the last day). */
export function periodDaysLeft(
  period: BudgetPeriod,
  now: Date = new Date()
): number {
  const year = now.getFullYear();
  if (period === "monthly") {
    const days = new Date(year, now.getMonth() + 1, 0).getDate();
    return days - now.getDate();
  }
  const start = Date.UTC(year, 0, 1);
  const today = Date.UTC(year, now.getMonth(), now.getDate());
  const days = (Date.UTC(year + 1, 0, 1) - start) / 86_400_000;
  return days - (Math.round((today - start) / 86_400_000) + 1);
}

/** Over once spending passes 100%, Close from 80%, else On track. */
export function budgetStatus(percentage: number): BudgetStatus {
  if (percentage > 100) return "over";
  if (percentage >= CLOSE_THRESHOLD) return "close";
  return "on_track";
}

/**
 * Spending relative to an even pace through the period: positive when spent
 * is ahead of where an even spend would be. In cents of the budget currency.
 */
export function paceDelta(
  budgeted: number,
  spent: number,
  elapsed: number
): number {
  return Math.round(spent - budgeted * elapsed);
}
