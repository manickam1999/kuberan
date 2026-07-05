import { useMemo } from "react";
import { useMonthlySummary } from "./use-transactions";

export interface ActiveMonth {
  /** First instant of the month, ISO 8601. */
  fromDate: string;
  /** Last instant of the month, ISO 8601. */
  toDate: string;
  /** e.g. "February 2026". */
  label: string;
  /** True when the resolved month is the real calendar month. */
  isCurrentMonth: boolean;
  /** Still resolving which month to show. */
  isLoading: boolean;
}

function monthRange(year: number, monthIndex: number) {
  const from = new Date(year, monthIndex, 1);
  const to = new Date(year, monthIndex + 1, 0, 23, 59, 59);
  return {
    fromDate: from.toISOString(),
    toDate: to.toISOString(),
    label: from.toLocaleDateString("en-US", {
      month: "long",
      year: "numeric",
    }),
  };
}

/**
 * Resolves the month that dashboard "this month" widgets should display.
 *
 * Personal finance data is often backfilled or imported, so the real calendar
 * month can be empty while recent months hold everything. This anchors those
 * widgets to the latest month that actually has spending, falling back to the
 * current month when data is fresh (or absent).
 */
export function useActiveMonth(): ActiveMonth {
  const { data, isLoading } = useMonthlySummary(12);

  return useMemo(() => {
    const now = new Date();
    const current = monthRange(now.getFullYear(), now.getMonth());

    // Latest month (data is ascending by month) with any expense activity.
    const withSpending = (data ?? []).filter((m) => m.expenses > 0);
    const latest = withSpending[withSpending.length - 1];

    if (!latest) {
      return { ...current, isCurrentMonth: true, isLoading };
    }

    const [year, month] = latest.month.split("-").map(Number);
    const isCurrentMonth =
      year === now.getFullYear() && month - 1 === now.getMonth();

    return {
      ...monthRange(year, month - 1),
      isCurrentMonth,
      isLoading,
    };
  }, [data, isLoading]);
}
