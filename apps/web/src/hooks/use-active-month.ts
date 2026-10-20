import { useMemo } from "react";
import { calendarMonthRange } from "@/lib/format";
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
    const currentRange = calendarMonthRange(now.getFullYear(), now.getMonth());
    // Convert local YYYY-MM-DD dates to ISO instants to preserve the existing
    // public contract (ActiveMonth.fromDate / toDate are ISO 8601 strings).
    const current = {
      fromDate: new Date(
        now.getFullYear(),
        now.getMonth(),
        1
      ).toISOString(),
      toDate: new Date(
        now.getFullYear(),
        now.getMonth() + 1,
        0,
        23,
        59,
        59
      ).toISOString(),
      label: currentRange.label,
    };

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
      fromDate: new Date(year, month - 1, 1).toISOString(),
      toDate: new Date(year, month, 0, 23, 59, 59).toISOString(),
      label: calendarMonthRange(year, month - 1).label,
      isCurrentMonth,
      isLoading,
    };
  }, [data, isLoading]);
}
