/**
 * Format cents as a currency string.
 * @param cents - Amount in cents (e.g., 1050 = $10.50)
 * @param currency - ISO 4217 currency code (default: "USD")
 */
export function formatCurrency(cents: number, currency = "MYR"): string {
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
 * Format a numeric percentage value.
 * @param value - Percentage value (e.g., 65.5 = "65.50%")
 */
export function formatPercentage(value: number): string {
  return `${value.toFixed(2)}%`;
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
