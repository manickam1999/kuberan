"use client";

import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { moneyText } from "@/lib/money-display";
import { cn } from "@/lib/utils";
import { Money } from "@/components/ui/money";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Investment } from "@/types/models";
import { holdingFigures } from "@/lib/holding-math";

export {
  holdingFigures,
  holdingReturnPct,
  type HoldingFigures,
} from "@/lib/holding-math";

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
    <Money
      amount={shown}
      currency={shownCurrency}
      sign="always"
      fxMissing={f.basePnl === null}
      className={cn(tone, className)}
    />
  );

  // Without a rate <Money> already explains it; with one, the tooltip adds the native figure.
  if (!foreign || f.basePnl === null) return text;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="cursor-help">{text}</span>
        </TooltipTrigger>
        <TooltipContent>
          {`In ${f.currency}: ${moneyText(f.nativePnl, f.currency, "always", false, hideBalances)}`}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
