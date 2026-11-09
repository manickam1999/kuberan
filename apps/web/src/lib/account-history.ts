import type { BalancePoint } from "@/types/api";

/** Chart ranges of the balance history card. */
export const HISTORY_RANGES = [
  { value: "30D", label: "30D", days: 30 },
  { value: "90D", label: "90D", days: 90 },
  { value: "1Y", label: "1Y", days: 365 },
] as const;

export type HistoryRange = (typeof HISTORY_RANGES)[number]["value"];

/** UTC calendar day (YYYY-MM-DD), the unit the API records balances in. */
export function utcDay(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** from/to query values covering the last `days` days, ending today (UTC). */
export function historyWindow(
  days: number,
  today: Date = new Date(),
): { from: string; to: string } {
  const from = new Date(today.getTime() - (days - 1) * 86_400_000);
  return { from: utcDay(from), to: utcDay(today) };
}

/**
 * Change over the recorded points: the move in cents and the percent of the
 * starting balance (null when it started at 0). Null with fewer than 2 points.
 */
export function historyChange(
  points: readonly Pick<BalancePoint, "balance">[],
): { delta: number; pct: number | null } | null {
  if (points.length < 2) return null;
  const first = points[0].balance;
  const delta = points[points.length - 1].balance - first;
  return {
    delta,
    pct: first === 0 ? null : (delta / Math.abs(first)) * 100,
  };
}

/** Chart points: label "Nov 3", value in major units. */
export function historyChartData(
  points: readonly BalancePoint[],
): { label: string; value: number }[] {
  return points.map((p) => ({
    label: new Date(`${p.day}T00:00:00Z`).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      timeZone: "UTC",
    }),
    value: p.balance / 100,
  }));
}
