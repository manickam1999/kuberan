"use client";

import { useMemo, useState } from "react";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { useAccountBalanceHistory } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import {
  HISTORY_RANGES,
  historyChange,
  historyChartData,
  historyWindow,
  type HistoryRange,
} from "@/lib/account-history";
import { formatMoney } from "@/lib/format";
import { cn } from "@/lib/utils";

const CHART_MARGIN = { top: 6, right: 10, bottom: 0, left: 10 } as const;

/**
 * An account's balance over time, in the account's own currency. The history is
 * recorded daily from the first pipeline run, so a new account has none yet:
 * the card renders nothing until there are two days to draw.
 */
export function BalanceHistoryCard({
  accountId,
  currency,
}: {
  accountId: string;
  currency: string;
}) {
  const [range, setRange] = useState<HistoryRange>("90D");
  const { hideBalances } = useAuth();
  const span = useMemo(() => {
    const days = HISTORY_RANGES.find((r) => r.value === range)?.days ?? 90;
    return historyWindow(days);
  }, [range]);
  const { data, isLoading, isError } = useAccountBalanceHistory(
    accountId,
    span,
  );

  const points = useMemo(() => data?.points ?? [], [data]);
  const change = historyChange(points);
  const chartData = useMemo(() => historyChartData(points), [points]);

  if (isLoading) {
    return <Skeleton className="h-[188px] w-full rounded-card" />;
  }
  // Nothing recorded yet (or too short to draw): the card stays out of the way.
  if (isError || (points.length < 2 && range === "90D")) return null;

  return (
    <Card className="gap-3 px-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="grid gap-0.5">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Balance history
          </span>
          {change && (
            <span
              className={cn(
                "text-xs",
                change.delta > 0 && "text-positive",
                change.delta < 0 && "text-negative",
                change.delta === 0 && "text-muted-foreground",
              )}
            >
              <Money
                amount={change.delta}
                currency={currency}
                sign="always"
                className="inline"
              />
              {change.pct !== null && (
                <>
                  {" "}
                  ({change.pct > 0 ? "+" : ""}
                  {change.pct.toFixed(1)}%)
                </>
              )}
              <span className="text-muted-foreground"> over the period</span>
            </span>
          )}
        </div>
        <Segmented
          aria-label="Balance history range"
          value={range}
          onValueChange={setRange}
          options={HISTORY_RANGES.map(({ value, label }) => ({ value, label }))}
        />
      </div>
      {points.length < 2 ? (
        <p className="text-muted-foreground py-8 text-center text-sm">
          Not enough history for this range yet.
        </p>
      ) : (
        <ThemedAreaChart
          data={chartData}
          seriesLabel="Balance"
          className="h-[120px] w-full"
          ditherColor="green"
          cleanColor="var(--chart-1)"
          margin={CHART_MARGIN}
          yPadding={Math.max(
            1,
            (Math.max(...chartData.map((d) => d.value)) -
              Math.min(...chartData.map((d) => d.value))) *
              0.1,
          )}
          valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
        />
      )}
    </Card>
  );
}
