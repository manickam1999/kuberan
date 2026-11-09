"use client";

import { AlertTriangle } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { formatMoney } from "@/lib/format";
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
  className?: string;
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

  const native = formatMoney(amount, currency, isMasked);

  if (isMasked) {
    return <span className={cn("tabular-nums", className)}>{native}</span>;
  }
  if (!foreign) {
    return <span className={cn("tabular-nums", className)}>{native}</span>;
  }

  const baseText = showBase
    ? `≈ ${formatMoney(baseAmount, baseCurrency)}`
    : null;

  const content = (
    <span
      className={cn(
        "tabular-nums",
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

