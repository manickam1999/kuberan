"use client";

import { AlertTriangle } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { formatMoney, MASKED_AMOUNT } from "@/lib/format";
import {
  MONEY_SIZE_CLASS,
  hasRaisedCurrency,
  moneyDisplay,
  type MoneySign,
  type MoneySize,
} from "@/lib/money-display";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export interface MoneyProps {
  /** Amount in minor units (cents) of `currency`. */
  amount: number;
  /** ISO 4217 currency of `amount`. */
  currency: string;
  /** The same amount in the base currency (e.g. `balance_base`, `base_amount`). */
  baseAmount?: number | null;
  /** Base currency; defaults to the user's reporting currency (`useBaseCurrency`). */
  baseCurrency?: string;
  /** `currency -> baseCurrency` rate as a decimal string, shown in the tooltip. */
  fxRate?: string | null;
  /** ISO 8601 date the rate applies to, shown in the tooltip. */
  fxDate?: string | null;
  /** The rate used is older than the staleness limit. */
  fxStale?: boolean;
  /** No rate exists: shows only the native amount plus a warning icon, never an `≈` figure. */
  fxMissing?: boolean;
  /** Force masking; when omitted, follows the user's `hide_balances` setting. */
  masked?: boolean;
  /**
   * `inline` (default): one line, `US$1,200.00 ≈ RM 5,512.00`, for tables.
   * `stacked`: the base figure on a muted second line, for cards.
   */
  layout?: "inline" | "stacked";
  /**
   * `auto` (default): a true minus (U+2212) for negative amounts only.
   * `always`: also `+` for positive amounts. `never`: the magnitude only.
   */
  sign?: MoneySign;
  /**
   * `hero | lg | md` set the figure in Source Serif 4 with a small raised
   * currency symbol; `sm` is plain `text-sm`. Omitted: inherits the
   * surrounding text (the previous behaviour).
   */
  size?: MoneySize;
  /** Dim the decimals (`.50`) so the whole units carry the eye. */
  dimCents?: boolean;
  /** Compact notation (`RM 1.2K`) for tight spaces. */
  compact?: boolean;
  className?: string;
}

interface AmountProps {
  amount: number;
  currency: string;
  sign: MoneySign;
  size?: MoneySize;
  dimCents: boolean;
  compact: boolean;
}

/** The styled number: symbol, sign, whole units and (optionally dimmed) cents. */
function Amount({ amount, currency, sign, size, dimCents, compact }: AmountProps) {
  const d = moneyDisplay(amount, currency, sign, compact);
  const raised = hasRaisedCurrency(size);
  const prefix = raised ? d.prefix.trim() : d.prefix;
  return (
    <>
      {prefix && (
        <span
          className={cn(
            raised &&
              "mr-[0.3em] align-[0.85em] font-sans text-[0.4em] font-normal tracking-normal"
          )}
        >
          {prefix}
        </span>
      )}
      {d.sign}
      {d.integer}
      {d.fraction &&
        (dimCents ? <span className="opacity-55">{d.fraction}</span> : d.fraction)}
      {d.suffix}
    </>
  );
}

function tooltipText(
  currency: string,
  baseCurrency: string,
  fxRate?: string | null,
  fxDate?: string | null,
  fxStale?: boolean,
  fxMissing?: boolean
): string {
  if (fxMissing) return `No exchange rate for ${currency} to ${baseCurrency}`;
  const parts: string[] = [];
  if (fxRate) parts.push(`1 ${currency} = ${fxRate} ${baseCurrency}`);
  if (fxDate) {
    const d = new Date(fxDate);
    if (!Number.isNaN(d.getTime())) {
      parts.push(`as of ${d.toLocaleDateString("en-US", { dateStyle: "medium" })}`);
    }
  }
  const line = parts.join(" ");
  if (fxStale) return line ? `${line}. Exchange rate is stale` : "Exchange rate is stale";
  return line || `Converted to ${baseCurrency}`;
}

/**
 * An amount in its own currency with the base-currency equivalent beside it
 * when the two differ (`US$1,200.00 ≈ RM 5,512.00`). A tooltip explains the
 * rate, its date, or that it is stale or missing. With `fxMissing` only the
 * native amount and a warning icon are shown. Masks per `hide_balances`.
 */
export function Money({
  amount,
  currency,
  baseAmount,
  baseCurrency: baseCurrencyProp,
  fxRate,
  fxDate,
  fxStale,
  fxMissing,
  masked,
  layout = "inline",
  sign = "auto",
  size,
  dimCents = false,
  compact = false,
  className,
}: MoneyProps) {
  const { hideBalances } = useAuth();
  const userBase = useBaseCurrency();
  const baseCurrency = baseCurrencyProp ?? userBase;
  const isMasked = masked ?? hideBalances;

  const foreign = currency !== baseCurrency;
  const showBase =
    foreign && !fxMissing && baseAmount !== undefined && baseAmount !== null;
  const warn = foreign && (fxMissing || fxStale);

  const sizeClass = size ? MONEY_SIZE_CLASS[size] : undefined;
  const native = (
    <Amount
      amount={amount}
      currency={currency}
      sign={sign}
      size={size}
      dimCents={dimCents}
      compact={compact}
    />
  );

  if (isMasked) {
    return (
      <span className={cn("tabular-nums", sizeClass, className)}>
        {MASKED_AMOUNT}
      </span>
    );
  }
  if (!foreign) {
    return (
      <span className={cn("tabular-nums", sizeClass, className)}>{native}</span>
    );
  }

  const baseText =
    showBase && baseAmount !== undefined && baseAmount !== null
      ? `≈ ${formatMoney(baseAmount, baseCurrency)}`
      : null;

  const content = (
    <span
      className={cn(
        "tabular-nums",
        sizeClass,
        layout === "stacked"
          ? "inline-flex flex-col items-start"
          : "inline-flex flex-wrap items-baseline gap-x-1.5",
        className
      )}
    >
      <span className="inline-flex items-center gap-1">
        {native}
        {warn && (
          <AlertTriangle
            className={cn(
              "size-3.5 shrink-0",
              fxMissing ? "text-negative" : "text-warning"
            )}
            aria-label={fxMissing ? "No exchange rate" : "Exchange rate is stale"}
          />
        )}
      </span>
      {baseText && (
        <span
          className={cn(
            "font-normal text-muted-foreground",
            layout === "stacked" ? "text-xs" : "text-[0.85em]"
          )}
        >
          {baseText}
        </span>
      )}
    </span>
  );

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="cursor-help">{content}</span>
        </TooltipTrigger>
        <TooltipContent>
          {tooltipText(currency, baseCurrency, fxRate, fxDate, fxStale, fxMissing)}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

