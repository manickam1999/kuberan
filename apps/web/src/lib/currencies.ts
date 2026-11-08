/**
 * Locale used for every currency display. `en-MY` renders MYR as `RM 1,234.50`
 * and USD as `US$1,234.50`, so the symbol is unambiguous for a Malaysian user.
 */
export const MONEY_LOCALE = "en-MY";

/**
 * Currencies offered in any currency select. Amounts are stored in cents (two
 * decimals), so zero-decimal currencies (JPY, KRW, VND, ...) are deliberately
 * absent. The API validates which of these it supports.
 */
export const REPORTING_CURRENCIES: { code: string; name: string }[] = [
  { code: "MYR", name: "Malaysian Ringgit" },
  { code: "USD", name: "US Dollar" },
  { code: "SGD", name: "Singapore Dollar" },
  { code: "GBP", name: "British Pound" },
  { code: "EUR", name: "Euro" },
  { code: "AUD", name: "Australian Dollar" },
  { code: "HKD", name: "Hong Kong Dollar" },
  { code: "CNY", name: "Chinese Yuan" },
  { code: "CAD", name: "Canadian Dollar" },
  { code: "NZD", name: "New Zealand Dollar" },
  { code: "CHF", name: "Swiss Franc" },
  { code: "THB", name: "Thai Baht" },
  { code: "IDR", name: "Indonesian Rupiah" },
  { code: "INR", name: "Indian Rupee" },
];

/**
 * The symbol `MONEY_LOCALE` uses for a currency code (`RM`, `US$`, `£`, or the
 * code itself when there is none, e.g. `SGD`). Falls back to the code for an
 * unknown or empty code.
 * @param code - ISO 4217 currency code
 */
export function currencySymbol(code: string): string {
  try {
    const parts = new Intl.NumberFormat(MONEY_LOCALE, {
      style: "currency",
      currency: code,
    }).formatToParts(0);
    return parts.find((p) => p.type === "currency")?.value ?? code;
  } catch {
    return code;
  }
}
