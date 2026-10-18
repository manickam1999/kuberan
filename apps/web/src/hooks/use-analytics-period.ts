"use client";

import { useMemo, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { calendarMonthRange, toLocalDateString } from "@/lib/format";
import { useActiveMonth } from "./use-active-month";

/** The analytics date window shared by all analytics-page cards. */
export interface AnalyticsPeriod {
  /** Local calendar date, YYYY-MM-DD, inclusive start. */
  fromDate: string;
  /** Local calendar date, YYYY-MM-DD, inclusive end. */
  toDate: string;
  /**
   * Human-readable label.
   * - Whole single month: "October 2026"
   * - Arbitrary range:    "Oct 1 - Nov 15, 2026"
   */
  label: string;
  /** "month" iff the range is exactly one whole calendar month; else "range". */
  mode: "month" | "range";
  /** True when NO url params are present (showing the latest-activity default). */
  isActiveMonthDefault: boolean;
  /** Number of distinct calendar months the range spans (>= 1). */
  monthCount: number;
  /** Navigate to a whole calendar month. monthIndex is 0-based. */
  setMonth(year: number, monthIndex: number): void;
  /** Navigate to an arbitrary local date range (YYYY-MM-DD each). */
  setRange(from: string, to: string): void;
  /** True while useActiveMonth is still resolving the default. */
  isLoading: boolean;
}

// ---------------------------------------------------------------------------
// Pure helpers (no React)
// ---------------------------------------------------------------------------

/** Return true if `dateStr` is a valid YYYY-MM-DD calendar date. */
function isValidLocalDate(dateStr: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) return false;
  const [year, month, day] = dateStr.split("-").map(Number);
  if (month < 1 || month > 12) return false;
  const d = new Date(year, month - 1, day);
  return (
    d.getFullYear() === year &&
    d.getMonth() === month - 1 &&
    d.getDate() === day
  );
}

/**
 * Return true when `fromDate` is the first of a month and `toDate` is the
 * last day of that same month.
 */
function isWholeCalendarMonth(fromDate: string, toDate: string): boolean {
  const [fy, fm, fd] = fromDate.split("-").map(Number);
  const [ty, tm, td] = toDate.split("-").map(Number);
  if (fy !== ty || fm !== tm) return false;
  if (fd !== 1) return false;
  // Last day of the month: day-0 of the next month.
  const lastDay = new Date(fy, fm, 0).getDate();
  return td === lastDay;
}

/**
 * Count the number of distinct calendar months between two YYYY-MM-DD dates
 * (both inclusive). Returns at least 1.
 */
function countCalendarMonths(fromDate: string, toDate: string): number {
  const [fy, fm] = fromDate.split("-").map(Number);
  const [ty, tm] = toDate.split("-").map(Number);
  return (ty - fy) * 12 + (tm - fm) + 1;
}

/**
 * Build a human-readable label for a date range.
 * - Whole single calendar month  -> "October 2026"
 * - Same-year range              -> "Oct 1 - Nov 15, 2026"
 * - Cross-year range             -> "Dec 1, 2025 - Jan 31, 2026"
 */
function buildLabel(fromDate: string, toDate: string): string {
  if (isWholeCalendarMonth(fromDate, toDate)) {
    const [year, month] = fromDate.split("-").map(Number);
    return new Date(year, month - 1, 1).toLocaleDateString("en-US", {
      month: "long",
      year: "numeric",
    });
  }

  const [fy, fm, fd] = fromDate.split("-").map(Number);
  const [ty, tm, td] = toDate.split("-").map(Number);
  const fromLocal = new Date(fy, fm - 1, fd);
  const toLocal = new Date(ty, tm - 1, td);

  const sameYear = fy === ty;

  if (sameYear) {
    const fromPart = fromLocal.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
    });
    const toPart = toLocal.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
    return `${fromPart} - ${toPart}`;
  }

  const fromPart = fromLocal.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
  const toPart = toLocal.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
  return `${fromPart} - ${toPart}`;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Manages the analytics date-range selection.
 *
 * URL (`?from=YYYY-MM-DD&to=YYYY-MM-DD`) is the source of truth. When no URL
 * params are present, the period seeds from `useActiveMonth()` — preserving
 * the existing "latest activity" behaviour — without rewriting the URL.
 *
 * Writes go through `router.replace` (not push) to keep the back button clean.
 */
export function useAnalyticsPeriod(): AnalyticsPeriod {
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeMonth = useActiveMonth();

  const rawFrom = searchParams.get("from");
  const rawTo = searchParams.get("to");
  const hasUrlParams = rawFrom !== null || rawTo !== null;

  // ---------------------------------------------------------------------------
  // Resolve the effective date window
  // ---------------------------------------------------------------------------
  const { fromDate, toDate } = useMemo<{
    fromDate: string;
    toDate: string;
  }>(() => {
    if (!hasUrlParams) {
      // No URL params: use the active-month default.
      // activeMonth.fromDate / toDate are ISO instants; convert to local dates.
      const fromLocal = toLocalDateString(new Date(activeMonth.fromDate));
      const toLocal = toLocalDateString(new Date(activeMonth.toDate));
      return { fromDate: fromLocal, toDate: toLocal };
    }

    // Validate params; fall back to active month on any error.
    const from = rawFrom ?? "";
    const to = rawTo ?? "";

    if (!isValidLocalDate(from) || !isValidLocalDate(to)) {
      const fromLocal = toLocalDateString(new Date(activeMonth.fromDate));
      const toLocal = toLocalDateString(new Date(activeMonth.toDate));
      return { fromDate: fromLocal, toDate: toLocal };
    }

    // Swap silently if reversed.
    if (from > to) {
      return { fromDate: to, toDate: from };
    }

    return { fromDate: from, toDate: to };
  }, [hasUrlParams, rawFrom, rawTo, activeMonth.fromDate, activeMonth.toDate]);

  // ---------------------------------------------------------------------------
  // Derived values
  // ---------------------------------------------------------------------------
  const mode = useMemo<"month" | "range">(
    () => (isWholeCalendarMonth(fromDate, toDate) ? "month" : "range"),
    [fromDate, toDate]
  );

  const monthCount = useMemo(
    () => countCalendarMonths(fromDate, toDate),
    [fromDate, toDate]
  );

  const label = useMemo(() => buildLabel(fromDate, toDate), [fromDate, toDate]);

  // ---------------------------------------------------------------------------
  // Writers
  // ---------------------------------------------------------------------------
  const setMonth = useCallback(
    (year: number, monthIndex: number) => {
      const range = calendarMonthRange(year, monthIndex);
      const next = new URLSearchParams(searchParams.toString());
      next.set("from", range.fromDate);
      next.set("to", range.toDate);
      router.replace(`?${next.toString()}`);
    },
    [router, searchParams]
  );

  const setRange = useCallback(
    (from: string, to: string) => {
      const [resolvedFrom, resolvedTo] = from <= to ? [from, to] : [to, from];
      const next = new URLSearchParams(searchParams.toString());
      next.set("from", resolvedFrom);
      next.set("to", resolvedTo);
      router.replace(`?${next.toString()}`);
    },
    [router, searchParams]
  );

  return {
    fromDate,
    toDate,
    label,
    mode,
    isActiveMonthDefault: !hasUrlParams,
    monthCount,
    setMonth,
    setRange,
    isLoading: activeMonth.isLoading,
  };
}
