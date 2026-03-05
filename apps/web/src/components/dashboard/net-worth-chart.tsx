"use client";

import { useMemo, useState } from "react";
import { AreaChart, Area, XAxis, YAxis, CartesianGrid } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from "@/components/ui/chart";
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
import { useIsMobile } from "@/hooks/use-mobile";
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

const chartConfig = {
  net_worth: { label: "Net Worth", color: "var(--chart-1)" },
} satisfies ChartConfig;

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
    date: formatLabel(s.recorded_at),
    net_worth: s.total_net_worth / 100,
  }));
}

export function NetWorthChart() {
  const isMobile = useIsMobile();
  const [period, setPeriod] = useState("1Y");

  const selectedOpt = useMemo(
    () =>
      PERIOD_OPTIONS.find((p) => p.value === period) ?? PERIOD_OPTIONS[5], // default 1Y
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

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Net Worth Over Time</CardTitle>
            <CardDescription>
              {latestNetWorth !== null
                ? `Current: ${formatCurrency(latestNetWorth)}`
                : "Portfolio value over time"}
            </CardDescription>
          </div>
          <Tabs value={period} onValueChange={setPeriod}>
            <TabsList>
              {PERIOD_OPTIONS.map((opt) => (
                <TabsTrigger key={opt.value} value={opt.value}>
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
          <ChartContainer
            config={chartConfig}
            className="h-[180px] md:h-[250px] w-full"
          >
            <AreaChart accessibilityLayer data={chartData}>
              <defs>
                <linearGradient
                  id="fillNetWorth"
                  x1="0"
                  y1="0"
                  x2="0"
                  y2="1"
                >
                  <stop
                    offset="0%"
                    stopColor="var(--color-net_worth)"
                    stopOpacity={0.4}
                  />
                  <stop
                    offset="95%"
                    stopColor="var(--color-net_worth)"
                    stopOpacity={0.1}
                  />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="date"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={isMobile ? 50 : 30}
              />
              <YAxis
                hide
                domain={["dataMin - 100", "dataMax + 100"]}
              />
              <ChartTooltip
                content={({ active, payload, label }) => {
                  if (!active || !payload?.length) return null;
                  const value = payload[0].value as number;
                  return (
                    <div className="border-border/50 bg-background rounded-lg border px-3 py-2 text-xs shadow-xl">
                      <div className="font-medium">{label}</div>
                      <div className="mt-1 font-mono font-medium tabular-nums">
                        {formatCurrency(value * 100)}
                      </div>
                    </div>
                  );
                }}
              />
              <Area
                type="monotone"
                dataKey="net_worth"
                stroke="var(--color-net_worth)"
                fill="url(#fillNetWorth)"
                strokeWidth={2}
              />
            </AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}
