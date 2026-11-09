import { formatMoneyParts, MASKED_AMOUNT, type MoneyParts } from "@/lib/format";

/** How `<Money>` shows the sign: minus only when negative, +/- always, or none. */
export type MoneySign = "auto" | "always" | "never";

/** Visual scale of a `<Money>` figure. `sm` is plain body text. */
export type MoneySize = "hero" | "lg" | "md" | "sm";

/** A displayable amount: `formatMoneyParts` with the sign resolved. */
export interface MoneyDisplay {
  prefix: string;
  /** `"−"`, `"+"` or `""`. */
  sign: string;
  integer: string;
  fraction: string;
  suffix: string;
}

/**
 * Resolve the pieces of an amount for display.
 * - `auto`: true minus for negative amounts only.
 * - `always`: `+` for positive amounts too (zero stays unsigned).
 * - `never`: the magnitude only.
 * @param compact - compact notation (`RM 1.2K`)
 */
export function moneyDisplay(
  cents: number,
  currency: string,
  sign: MoneySign = "auto",
  compact = false
): MoneyDisplay {
  const p: MoneyParts = formatMoneyParts(cents, currency, compact);
  let s = "";
  if (sign !== "never") {
    if (cents < 0) s = p.minus;
    else if (sign === "always" && cents > 0) s = "+";
  }
  return {
    prefix: p.prefix,
    sign: s,
    integer: p.integer,
    fraction: p.fraction,
    suffix: p.suffix,
  };
}

/** Plain-text form of {@link moneyDisplay}, matching what `<Money>` renders. */
export function moneyText(
  cents: number,
  currency: string,
  sign: MoneySign = "auto",
  compact = false,
  masked = false
): string {
  if (masked) return MASKED_AMOUNT;
  const d = moneyDisplay(cents, currency, sign, compact);
  return `${d.prefix}${d.sign}${d.integer}${d.fraction}${d.suffix}`;
}

/** Tailwind classes per size. The figure face is Source Serif 4 (`.font-figures`). */
export const MONEY_SIZE_CLASS: Record<MoneySize, string> = {
  hero: "font-figures text-[40px] leading-none tracking-[-0.02em] sm:text-[54px]",
  lg: "font-figures text-[32px] leading-none tracking-[-0.015em]",
  md: "font-figures text-2xl leading-[1.1] tracking-[-0.01em]",
  sm: "text-sm",
};

/** Sizes where the currency symbol is set small and raised, like the prototype. */
export function hasRaisedCurrency(size: MoneySize | undefined): boolean {
  return size === "hero" || size === "lg" || size === "md";
}
