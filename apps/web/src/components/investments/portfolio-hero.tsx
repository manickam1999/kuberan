"use client";

import { useMemo, useState } from "react";
import { TrendingDown, TrendingUp } from "lucide-react";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { FxNotice } from "@/components/ui/fx-notice";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { usePortfolioSnapshots } from "@/hooks/use-portfolio-snapshots";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { formatDate, formatMoney } from "@/lib/format";
import { signedPercent } from "@/lib/price-series";
import { useChartTheme } from "@/providers/chart-theme-provider";
import type { PortfolioSummary } from "@/types/models";

const PERIODS = [
  { value: "1M", label: "1M", months: 1 },
  { value: "3M", label: "3M", months: 3 },
  { value: "6M", label: "6M", months: 6 },
  { value: "1Y", label: "1Y", months: 12 },
  { value: "ALL", label: "All", months: 120 },
] as const;

type Period = (typeof PERIODS)[number]["value"];

function getDateRange(months: number) {
  const to = new Date();
  const from = new Date();
  from.setMonth(from.getMonth() - months);
  // The backend compares against midnight UTC, so include the next day.
  const toNextDay = new Date(to);
  toNextDay.setDate(toNextDay.getDate() + 1);
  return {
    from_date: from.toISOString().split("T")[0],
    to_date: toNextDay.toISOString().split("T")[0],
  };
}

/**
 * The Investments hero: total value, unrealised gain and the value history,
 * with a dashed line at the cost basis so the gain is visible. Everything is in
 * the portfolio's base currency.
 */
export function PortfolioHero({ portfolio }: { portfolio: PortfolioSummary }) {
  const [period, setPeriod] = useState<Period>("1Y");
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const { chartTheme } = useChartTheme();
  const currency = portfolio.currency || baseCurrency;

  const { from_date, to_date } = useMemo(() => {
    const opt = PERIODS.find((p) => p.value === period);
    return getDateRange(opt?.months ?? 12);
  }, [period]);

  const { data: snapshotsData, isLoading } = usePortfolioSnapshots({
    from_date,
    to_date,
    page_size: 100, // backend max
  });

  const chartData = useMemo(() => {
    if (!snapshotsData?.data) return [];
    // The API returns newest first; the chart reads oldest to newest.
    return snapshotsData.data
      .map((s) => ({
        label: formatDate(s.recorded_at),
        value: s.investment_value / 100,
      }))
      .reverse();
  }, [snapshotsData]);

  const anyFxMissing = snapshotsData?.data?.some((s) => s.fx_missing) ?? false;
  const gain = portfolio.total_gain_loss;
  const up = gain >= 0;
  const showCostLine = chartTheme !== "dither" && portfolio.total_cost_basis > 0;

  return (
    <Card className="gap-4">
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="text-muted-foreground text-xs font-medium tracking-wider whitespace-nowrap uppercase">
            Portfolio value
          </span>
          <Segmented
            aria-label="Chart range"
            value={period}
            onValueChange={setPeriod}
            options={PERIODS.map(({ value, label }) => ({ value, label }))}
          />
        </div>

        <div className="space-y-2">
          <div>
            <Money
              amount={portfolio.total_value}
              currency={currency}
              size="hero"
              dimCents
            />
          </div>
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <Badge tone={up ? "pos" : "neg"}>
              {up ? <TrendingUp aria-hidden /> : <TrendingDown aria-hidden />}
              <Money amount={gain} currency={currency} sign="always" />
              <span aria-hidden>·</span>
              {signedPercent(portfolio.gain_loss_pct)}
            </Badge>
            <span className="text-muted-foreground text-sm">
              unrealised ·{" "}
              <Money
                amount={portfolio.total_realized_gain_loss}
                currency={currency}
                sign="always"
              />{" "}
              realised
            </span>
          </div>
        </div>

        {isLoading ? (
          <Skeleton className="h-[220px] w-full md:h-[260px]" />
        ) : chartData.length === 0 ? (
          <p className="text-muted-foreground py-10 text-center text-sm">
            No value history yet. Snapshots are taken each day by the price
            update.
          </p>
        ) : (
          <ThemedAreaChart
            data={chartData}
            seriesLabel="Portfolio value"
            className="h-[220px] w-full md:h-[260px]"
            ditherColor="green"
            cleanColor="var(--chart-1)"
            baseline={
              portfolio.total_cost_basis > 0
                ? portfolio.total_cost_basis / 100
                : undefined
            }
            valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
          />
        )}

        {showCostLine && chartData.length > 0 && (
          <p className="text-muted-foreground text-xs">
            Dashed line: what you put in (
            <Money amount={portfolio.total_cost_basis} currency={currency} />)
          </p>
        )}
        <FxNotice
          unconverted={portfolio.unconverted}
          fxStale={portfolio.fx_stale}
        />
        {anyFxMissing && <FxNotice fxMissing variant="inline" />}
      </CardContent>
    </Card>
  );
}
