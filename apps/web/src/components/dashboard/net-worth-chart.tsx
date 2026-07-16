"use client";

import { useMemo, useState } from "react";
import { ArrowUpRight, ArrowDownRight } from "lucide-react";
import { ThemedAreaChart } from "@/components/charts/themed-area-chart";
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
import {
  formatCurrency,
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

export function NetWorthChart() {
  const [period, setPeriod] = useState("1W");

  const selectedOpt = useMemo(
    () =>
      PERIOD_OPTIONS.find((p) => p.value === period) ?? PERIOD_OPTIONS[1], // default 1W
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

  const chartData = useMemo(() => {
    if (isGrouped) {
      // Grouped response: { data: [...] } sorted chronologically (oldest first)
      const data = groupedQuery.data?.data;
      if (!data) return [];
      return toChartData(data, selectedOpt.formatLabel);
    }
    // Raw paginated response: { data: [...], page, ... } sorted DESC (newest first)
    const data = rawQuery.data?.data;
    if (!data) return [];
    return toChartData(data, selectedOpt.formatLabel).reverse();
  }, [
    isGrouped,
    groupedQuery.data,
    rawQuery.data,
    selectedOpt.formatLabel,
  ]);

  const latestNetWorth = useMemo(() => {
    if (isGrouped) {
      const data = groupedQuery.data?.data;
      if (!data || data.length === 0) return null;
      return data[data.length - 1].total_net_worth; // last = newest (ASC order)
    }
    const data = rawQuery.data?.data;
    if (!data || data.length === 0) return null;
    return data[0].total_net_worth; // first = newest (DESC order)
  }, [isGrouped, groupedQuery.data, rawQuery.data]);

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
              <CardTitle className="money text-3xl font-semibold sm:text-4xl">
                {latestNetWorth !== null
                  ? formatCurrency(latestNetWorth)
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
                  {formatCurrency(periodChange.deltaCents)} (
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
          <Tabs value={period} onValueChange={setPeriod}>
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
            valueFormatter={(v) => formatCurrency(v * 100)}
          />
        )}
      </CardContent>
    </Card>
  );
}
