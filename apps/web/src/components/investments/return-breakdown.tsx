import { Money } from "@/components/ui/money";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { PortfolioSummary } from "@/types/models";

function Row({
  label,
  hint,
  amount,
  currency,
  strong,
}: {
  label: string;
  hint: string;
  amount: number;
  currency: string;
  strong?: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5">
      <div className="min-w-0">
        <p className={cn("text-sm", strong && "font-medium")}>{label}</p>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>
      <Money
        amount={amount}
        currency={currency}
        sign="always"
        className={cn(
          "shrink-0 text-sm",
          strong && "font-medium",
          amount > 0 ? "text-positive" : amount < 0 ? "text-negative" : ""
        )}
      />
    </div>
  );
}

/**
 * Where the unrealised gain came from: the securities' prices or the exchange
 * rate on the cost. Price return + FX return = unrealised gain (base currency).
 */
export function ReturnBreakdown({ portfolio }: { portfolio: PortfolioSummary }) {
  const currency = portfolio.currency;
  return (
    <Card className="gap-2">
      <CardHeader>
        <CardTitle className="text-base">Return</CardTitle>
        <CardDescription>Price movement vs currency movement</CardDescription>
      </CardHeader>
      <CardContent className="divide-border/60 divide-y">
        <Row
          label="Price return"
          hint="from prices moving"
          amount={portfolio.total_price_return}
          currency={currency}
        />
        <Row
          label="FX return"
          hint="from exchange rates moving"
          amount={portfolio.total_fx_return}
          currency={currency}
        />
        <Row
          label="Unrealised gain"
          hint="open positions"
          amount={portfolio.total_gain_loss}
          currency={currency}
          strong
        />
        <Row
          label="Realised gain"
          hint="closed and part-closed positions"
          amount={portfolio.total_realized_gain_loss}
          currency={currency}
        />
      </CardContent>
    </Card>
  );
}
