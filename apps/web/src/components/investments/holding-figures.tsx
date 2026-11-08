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
import type { Investment } from "@/types/models";

/**
 * Per-holding figures. Native figures (security currency) and base figures
 * (user's reporting currency) are never combined: each P&L subtracts a cost
 * basis from a value in the same currency.
 */
export interface HoldingFigures {
  /** Security currency. */
  currency: string;
  /** quantity x current_price_native, in `currency`. */
  nativeValue: number;
  /** quantity x current_price, in the base currency; null without an FX rate. */
  baseValue: number | null;
  /** nativeValue - cost_basis, in `currency`. */
  nativePnl: number;
  /** baseValue - cost_basis_base, in the base currency; null without an FX rate. */
  basePnl: number | null;
}

/** Computes the native and base value / unrealized P&L of an open holding. */
export function holdingFigures(inv: Investment): HoldingFigures {
  const nativeValue = Math.round(inv.quantity * inv.current_price_native);
  const baseValue = inv.fx_missing
    ? null
    : Math.round(inv.quantity * inv.current_price);
  return {
    currency: inv.price_currency || inv.currency,
    nativeValue,
    baseValue,
    nativePnl: nativeValue - inv.cost_basis,
    basePnl: baseValue === null ? null : baseValue - inv.cost_basis_base,
  };
}

/** Signed money text: "+US$1.00" / "-US$1.00" (the sign comes from the amount). */
export function formatSigned(
  cents: number,
  currency: string,
  masked: boolean
): string {
  if (masked) return formatMoney(cents, currency, true);
  return `${cents > 0 ? "+" : ""}${formatMoney(cents, currency)}`;
}

/**
 * Unrealized P&L of a holding in the BASE currency; the tooltip shows the
 * native P&L when the security is in another currency. Without an FX rate the
 * native P&L is shown with a warning instead.
 */
export function UnrealizedPnl({
  investment,
  className,
}: {
  investment: Investment;
  className?: string;
}) {
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const f = holdingFigures(investment);
  const foreign = f.currency !== baseCurrency;

  const shown = f.basePnl ?? f.nativePnl;
  const shownCurrency = f.basePnl === null ? f.currency : baseCurrency;
  const tone = shown >= 0 ? "text-positive" : "text-negative";

  const text = (
    <span className={cn("tabular-nums", tone, className)}>
      {formatSigned(shown, shownCurrency, hideBalances)}
      {f.basePnl === null && (
        <AlertTriangle
          className="ml-1 inline size-3.5 text-negative"
          aria-label="No exchange rate"
        />
      )}
    </span>
  );

  if (!foreign) return text;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="cursor-help">{text}</span>
        </TooltipTrigger>
        <TooltipContent>
          {f.basePnl === null
            ? `No exchange rate for ${f.currency} to ${baseCurrency}`
            : `In ${f.currency}: ${formatSigned(f.nativePnl, f.currency, hideBalances)}`}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
