/** Placeholder shown in place of an amount when balance visibility is hidden. */
export const MASKED_AMOUNT = "****";

/**
 * Format cents as a currency string.
 * @param cents - Amount in cents (e.g., 1050 = $10.50)
 * @param currency - ISO 4217 currency code (default: "USD")
 * @param masked - If true, returns {@link MASKED_AMOUNT} instead of the formatted value
 */
export function formatCurrency(
  cents: number,
  currency = "MYR",
  masked = false
): string {
  if (masked) return MASKED_AMOUNT;
  const dollars = cents / 100;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currency || "MYR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(dollars);
}

/**
 * Format an ISO 8601 date string as a human-readable date.
 * Extracts the date portion and constructs a local Date to avoid
 * timezone-induced off-by-one errors (e.g. midnight UTC appearing
 * as the previous day in UTC- timezones).
 * @param iso - ISO 8601 date string (e.g. "2026-02-15T00:00:00Z")
 */
export function formatDate(iso: string): string {
  const dateStr = iso.split("T")[0];
  const [year, month, day] = dateStr.split("-").map(Number);
  const local = new Date(year, month - 1, day);
  return local.toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

/**
 * Format an ISO 8601 date string as a human-readable date + time.
 * @param iso - ISO 8601 date string
 */
export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

/**
 * Format an ISO 8601 date string as a time only (e.g. "2:30 PM").
 * Useful for intraday charts (1D timeframe).
 * @param iso - ISO 8601 date string
 */
export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
  });
}

/**
 * Format an ISO 8601 date string as a short weekday + time (e.g. "Mon 2:00 PM").
 * Useful for weekly charts (1W timeframe).
 * @param iso - ISO 8601 date string
 */
export function formatShortDateTime(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    weekday: "short",
    hour: "numeric",
    minute: "2-digit",
  });
}

/**
 * Format a whole-unit number compactly for chart axes (e.g. 181000 -> "181K").
 * Input is in display units (dollars), not cents.
 * @param value - Numeric value in whole units
 */
export function formatCompact(value: number): string {
  return new Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

/**
 * Format a numeric percentage value.
 * @param value - Percentage value (e.g., 65.5 = "65.50%")
 */
export function formatPercentage(value: number): string {
  return `${value.toFixed(2)}%`;
}

/**
 * Format a Date as YYYY-MM-DD using local date components.
 * Avoids the UTC-shift bug in `date.toISOString().split("T")[0]`, which
 * can report the wrong calendar day for timezones ahead of UTC (e.g. a
 * user in UTC+8 sees yesterday's date for the first ~8 hours of their day).
 * @param date - Date to format (default: now)
 */
export function toLocalDateString(date: Date = new Date()): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/**
 * "March 2026" for an ISO date, without timezone-induced day shifts.
 * @param iso - A YYYY-MM-DD or RFC 3339 date string
 */
export function formatMonthYear(iso: string): string {
  const [year, month] = iso.split("T")[0].split("-").map(Number);
  return new Date(year, month - 1, 1).toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  });
}

/**
 * Convert a date-only string (YYYY-MM-DD) to RFC 3339 format (YYYY-MM-DDT00:00:00Z).
 * If the string already contains "T" (i.e., is already RFC 3339), it is returned as-is.
 * @param dateStr - A date string in YYYY-MM-DD or RFC 3339 format
 */
export function toRFC3339(dateStr: string): string {
  if (dateStr.includes("T")) {
    return dateStr;
  }
  return `${dateStr}T00:00:00Z`;
}

/**
 * Convert a local calendar date string (YYYY-MM-DD) to the RFC3339 UTC instant
 * at the start of that day in the browser's local timezone. The backend parses
 * a bare YYYY-MM-DD as UTC midnight, which is wrong for any timezone ahead of
 * UTC (e.g. a UTC+8 user's whole local day would be shifted into "yesterday"
 * for the query range) — sending the real UTC instant avoids that mismatch.
 * Strings that are already RFC3339 (contain "T") are passed through unchanged.
 * @param dateStr - Local calendar date (YYYY-MM-DD) or an RFC3339 string
 */
export function localDayStartUTC(dateStr: string): string {
  if (dateStr.includes("T")) {
    return dateStr;
  }
  const [year, month, day] = dateStr.split("-").map(Number);
  return new Date(year, month - 1, day, 0, 0, 0, 0).toISOString();
}

/**
 * Convert a local calendar date string (YYYY-MM-DD) to the RFC3339 UTC instant
 * at the end of that day (23:59:59.999) in the browser's local timezone.
 * Strings that are already RFC3339 (contain "T") are passed through unchanged.
 * See {@link localDayStartUTC} for why this is needed instead of a bare date.
 * @param dateStr - Local calendar date (YYYY-MM-DD) or an RFC3339 string
 */
export function localDayEndUTC(dateStr: string): string {
  if (dateStr.includes("T")) {
    return dateStr;
  }
  const [year, month, day] = dateStr.split("-").map(Number);
  return new Date(year, month - 1, day, 23, 59, 59, 999).toISOString();
}

// ---- Fixed deposit enum labels ----

import type { PayoutType, PayoutFrequency, Compounding } from "@/types/models";

const PAYOUT_TYPE_LABELS: Record<PayoutType, string> = {
  cumulative: "Cumulative",
  periodic: "Periodic",
};

const PAYOUT_FREQUENCY_LABELS: Record<PayoutFrequency, string> = {
  monthly: "Monthly",
  quarterly: "Quarterly",
  semiannual: "Semiannual",
  annual: "Annual",
};

const COMPOUNDING_LABELS: Record<Compounding, string> = {
  monthly: "Monthly",
  quarterly: "Quarterly",
  semiannual: "Semiannual",
  annual: "Annual",
};

/** Human-readable label for a fixed deposit payout type. */
export function formatPayoutType(value: PayoutType): string {
  return PAYOUT_TYPE_LABELS[value] ?? value;
}

/** Human-readable label for a fixed deposit payout frequency. */
export function formatPayoutFrequency(value: PayoutFrequency): string {
  return PAYOUT_FREQUENCY_LABELS[value] ?? value;
}

/** Human-readable label for a fixed deposit compounding frequency. */
export function formatCompounding(value: Compounding): string {
  return COMPOUNDING_LABELS[value] ?? value;
}

/**
 * Compute the first and last local calendar dates (YYYY-MM-DD) of a given month,
 * plus a human-readable label like "October 2026".
 *
 * This is the canonical month-range helper shared by `useActiveMonth` and
 * `useAnalyticsPeriod`. Dates are local calendar strings, not ISO instants,
 * to avoid the UTC-shift class of bug documented in `localDayStartUTC`.
 *
 * @param year - Four-digit calendar year
 * @param monthIndex - 0-based month index (0 = January, 11 = December)
 */
export function calendarMonthRange(
  year: number,
  monthIndex: number
): { fromDate: string; toDate: string; label: string } {
  const first = new Date(year, monthIndex, 1);
  // Day 0 of the next month is the last day of this month.
  const last = new Date(year, monthIndex + 1, 0);
  const label = first.toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  });
  return {
    fromDate: toLocalDateString(first),
    toDate: toLocalDateString(last),
    label,
  };
}
