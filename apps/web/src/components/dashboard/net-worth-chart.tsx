"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowUpRight, ArrowDownRight } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  usePortfolioSnapshots,
  useGroupedPortfolioSnapshots,
} from "@/hooks/use-portfolio-snapshots";
import { FxNotice } from "@/components/ui/fx-notice";
import {
  formatMoney,
  formatDate,
  formatTime,
  formatShortDateTime,
} from "@/lib/format";
import type { PortfolioSnapshot } from "@/types/models";

interface PeriodOption {
  readonly value: string;
  readonly label: string;
  readonly months: number;
  readonly days: number;
  readonly groupBy: "day" | "hour" | undefined;
  readonly formatLabel: (iso: string) => string;
}

const PERIOD_OPTIONS: readonly PeriodOption[] = [
  {
    value: "1D",
    label: "1D",
    months: 0,
    days: 1,
    groupBy: undefined,
    formatLabel: formatTime,
  },
  {
    value: "1W",
    label: "1W",
    months: 0,
    days: 7,
    groupBy: "hour",
    formatLabel: formatShortDateTime,
  },
  {
    value: "1M",
    label: "1M",
    months: 1,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
  },
  {
    value: "3M",
    label: "3M",
    months: 3,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
  },
  {
    value: "6M",
    label: "6M",
    months: 6,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
  },
  {
    value: "1Y",
    label: "1Y",
    months: 12,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
  },
  {
    value: "ALL",
    label: "ALL",
    months: 120,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
  },
] as const;

/** Format a Date as YYYY-MM-DD using local date components (avoids UTC shift). */
function toLocalDateString(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function getDateRange(opt: PeriodOption) {
  const to = new Date();
  const from = new Date();
  if (opt.months > 0) {
    from.setMonth(from.getMonth() - opt.months);
  } else {
    from.setDate(from.getDate() - opt.days);
  }

  // Add 1 day to 'to' date to ensure we include all snapshots from the end date
  // (backend uses <= comparison with midnight UTC, so we need to go to next day)
  const toNextDay = new Date(to);
  toNextDay.setDate(toNextDay.getDate() + 1);

  return {
    from_date: toLocalDateString(from),
    to_date: toLocalDateString(toNextDay),
  };
}

/** Transform raw snapshot array into chart-ready data. */
function toChartData(
  snapshots: PortfolioSnapshot[],
  formatLabel: (iso: string) => string
) {
  return snapshots.map((s) => ({
    label: formatLabel(s.recorded_at),
    value: s.total_net_worth / 100,
  }));
}

/**
 * Periods tried, in order, until the user picks one. 6M is the default; a new
 * account has only a few days of snapshots, which 6M (grouped by day) shows as
 * a handful of points, so step down to finer grouping until the line has shape.
 */
const AUTO_PERIODS = ["6M", "1W", "1D"] as const;
const MIN_AUTO_POINTS = 8;

export function NetWorthChart() {
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  // null until the user picks a period; until then AUTO_PERIODS decides.
  const [chosenPeriod, setChosenPeriod] = useState<string | null>(null);
  const [autoStep, setAutoStep] = useState(0);
  const period = chosenPeriod ?? AUTO_PERIODS[autoStep];

  const selectedOpt = useMemo(
    () =>
      PERIOD_OPTIONS.find((p) => p.value === period) ?? PERIOD_OPTIONS[4], // default 6M
    [period]
  );

  const { from_date, to_date } = useMemo(
    () => getDateRange(selectedOpt),
    [selectedOpt]
  );

  // For 1D (no group_by): fetch raw paginated snapshots with a large enough page_size.
  // 48 snapshots/day at 30-min intervals, pad a bit for safety.
  const rawQuery = usePortfolioSnapshots({
    from_date,
    to_date,
    page_size: 50,
  });

  // For 1W+ (with group_by): fetch downsampled snapshots.
  const groupedQuery = useGroupedPortfolioSnapshots({
    from_date,
    to_date,
    group_by: selectedOpt.groupBy,
  });

  const isGrouped = !!selectedOpt.groupBy;
  const activeQuery = isGrouped ? groupedQuery : rawQuery;
  const { isLoading, error } = activeQuery;

  // Snapshots oldest first, whichever query served them.
  const snapshots = useMemo<PortfolioSnapshot[]>(() => {
    if (isGrouped) return groupedQuery.data?.data ?? [];
    // Raw paginated response is newest first.
    return [...(rawQuery.data?.data ?? [])].reverse();
  }, [isGrouped, groupedQuery.data, rawQuery.data]);

  // The reporting currency can change, so snapshots may span currencies. The
  // chart plots the newest run of one currency. Earlier snapshots in another
  // currency are not charted, so the line never joins two currencies.
  const run = useMemo(() => {
    const last = snapshots[snapshots.length - 1];
    if (!last) return null;
    let start = snapshots.length - 1;
    while (start > 0 && snapshots[start - 1].currency === last.currency) {
      start--;
    }
    const before = start > 0 ? snapshots[start - 1] : null;
    return {
      currency: last.currency,
      points: snapshots.slice(start),
      changedAt: before ? snapshots[start].recorded_at : null,
      previousCurrency: before ? before.currency : null,
    };
  }, [snapshots]);

  const currency = run?.currency ?? baseCurrency;
  const anyFxMissing = run?.points.some((s) => s.fx_missing) ?? false;

  const chartData = useMemo(() => {
    if (!run) return [];
    return toChartData(run.points, selectedOpt.formatLabel);
  }, [run, selectedOpt.formatLabel]);

  useEffect(() => {
    if (chosenPeriod !== null || isLoading || error) return;
    if (chartData.length < MIN_AUTO_POINTS && autoStep < AUTO_PERIODS.length - 1) {
      setAutoStep((s) => s + 1);
    }
  }, [chosenPeriod, isLoading, error, chartData.length, autoStep]);

  const latestPoint = run?.points[run.points.length - 1];
  const latestNetWorth = latestPoint ? latestPoint.total_net_worth : null;

  // Change over the selected window (first vs last point), in cents.
  const periodChange = useMemo(() => {
    if (chartData.length < 2 || latestNetWorth === null) return null;
    const startCents = chartData[0].value * 100;
    const deltaCents = latestNetWorth - startCents;
    const pct = startCents !== 0 ? (deltaCents / Math.abs(startCents)) * 100 : 0;
    return { deltaCents, pct };
  }, [chartData, latestNetWorth]);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-36" />
          <Skeleton className="h-4 w-48" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[300px] w-full" />
        </CardContent>
      </Card>
    );
  }

  const changeUp = periodChange ? periodChange.deltaCents >= 0 : true;

  return (
    <Card className="surface-glow">
      <CardHeader>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-1.5">
            <CardDescription className="text-xs font-medium uppercase tracking-wide">
              Net worth
            </CardDescription>
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <CardTitle className="money font-figures text-3xl font-normal sm:text-[2.75rem]">
                {latestNetWorth !== null
                  ? formatMoney(latestNetWorth, currency, hideBalances)
                  : "—"}
              </CardTitle>
              {periodChange && (
                <span
                  className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${
                    changeUp
                      ? "bg-positive-muted text-positive"
                      : "bg-negative-muted text-negative"
                  }`}
                >
                  {changeUp ? (
                    <ArrowUpRight className="size-3.5" />
                  ) : (
                    <ArrowDownRight className="size-3.5" />
                  )}
                  {changeUp ? "+" : ""}
                  {formatMoney(periodChange.deltaCents, currency, hideBalances)} (
                  {changeUp ? "+" : ""}
                  {periodChange.pct.toFixed(1)}%)
                </span>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              {selectedOpt.label === "ALL"
                ? "All time"
                : `Past ${selectedOpt.label}`}
            </p>
          </div>
          <Tabs value={period} onValueChange={setChosenPeriod}>
            <TabsList className="h-8">
              {PERIOD_OPTIONS.map((opt) => (
                <TabsTrigger key={opt.value} value={opt.value} className="px-2 text-xs">
                  {opt.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
      </CardHeader>
      <CardContent>
        {error ? (
          <div className="flex flex-col items-center justify-center py-10 text-destructive">
            <p className="text-sm">
              Error loading snapshot data:{" "}
              {error instanceof Error ? error.message : "Unknown error"}
            </p>
          </div>
        ) : chartData.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
            <p className="text-sm">
              No snapshot data available. Snapshots are generated periodically by
              the pipeline.
            </p>
          </div>
        ) : (
          <ThemedAreaChart
            data={chartData}
            seriesLabel="Net Worth"
            className="h-[190px] md:h-[230px] w-full"
            ditherColor="green"
            cleanColor="var(--chart-1)"
            valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
          />
        )}
        {run?.changedAt && run.previousCurrency && (
          <p className="mt-2 text-xs text-muted-foreground">
            Base currency changed to {run.currency} on{" "}
            {formatDate(run.changedAt)}. Earlier snapshots in{" "}
            {run.previousCurrency} are not charted, so the line does not join
            the two.
          </p>
        )}
        <FxNotice fxMissing={anyFxMissing} variant="inline" className="mt-2" />
      </CardContent>
    </Card>
  );
}
