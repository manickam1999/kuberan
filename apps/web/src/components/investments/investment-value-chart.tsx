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
import { usePortfolioSnapshots } from "@/hooks/use-portfolio-snapshots";
import { formatCurrency, formatDate } from "@/lib/format";

const PERIOD_OPTIONS = [
  { value: "1M", label: "1M", months: 1 },
  { value: "3M", label: "3M", months: 3 },
  { value: "6M", label: "6M", months: 6 },
  { value: "1Y", label: "1Y", months: 12 },
  { value: "ALL", label: "ALL", months: 120 },
] as const;

const chartConfig = {
  investment_value: { label: "Investment Value", color: "var(--chart-1)" },
} satisfies ChartConfig;

function getDateRange(months: number) {
  const to = new Date();
  const from = new Date();
  from.setMonth(from.getMonth() - months);
  
  // Add 1 day to 'to' date to ensure we include all snapshots from the end date
  // (backend uses <= comparison with midnight UTC, so we need to go to next day)
  const toNextDay = new Date(to);
  toNextDay.setDate(toNextDay.getDate() + 1);
  
  return {
    from_date: from.toISOString().split("T")[0],
    to_date: toNextDay.toISOString().split("T")[0],
  };
}

export function InvestmentValueChart() {
  const [period, setPeriod] = useState("1Y");

  const { from_date, to_date } = useMemo(() => {
    const opt = PERIOD_OPTIONS.find((p) => p.value === period);
    return getDateRange(opt?.months ?? 12);
  }, [period]);

  const { data: snapshotsData, isLoading } = usePortfolioSnapshots({
    from_date,
    to_date,
    page_size: 100, // Backend max is 100
  });

  const chartData = useMemo(() => {
    if (!snapshotsData?.data) return [];
    // Backend returns data in DESC order (newest first)
    // Reverse it for chart display (oldest to newest, left to right)
    return snapshotsData.data
      .map((s) => ({
        date: formatDate(s.recorded_at),
        investment_value: s.investment_value / 100,
      }))
      .reverse();
  }, [snapshotsData]);

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
    <Card className="h-full">
      <CardHeader>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <CardTitle>Investment Value</CardTitle>
            <CardDescription>Portfolio value over time</CardDescription>
          </div>
          <Tabs value={period} onValueChange={setPeriod}>
            <TabsList className="w-full sm:w-auto">
              {PERIOD_OPTIONS.map((opt) => (
                <TabsTrigger 
                  key={opt.value} 
                  value={opt.value}
                  className="flex-1 sm:flex-initial"
                >
                  {opt.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
      </CardHeader>
      <CardContent>
        {chartData.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
            <p className="text-sm">
              No snapshot data available. Snapshots are generated periodically by
              the pipeline.
            </p>
          </div>
        ) : (
          <ChartContainer
            config={chartConfig}
            className="h-[220px] md:h-[280px] w-full"
          >
            <AreaChart accessibilityLayer data={chartData}>
              <defs>
                <linearGradient
                  id="fillInvestmentValue"
                  x1="0"
                  y1="0"
                  x2="0"
                  y2="1"
                >
                  <stop
                    offset="0%"
                    stopColor="var(--color-investment_value)"
                    stopOpacity={0.3}
                  />
                  <stop
                    offset="95%"
                    stopColor="var(--color-investment_value)"
                    stopOpacity={0.05}
                  />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="date"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
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
                dataKey="investment_value"
                stroke="var(--color-investment_value)"
                fill="url(#fillInvestmentValue)"
                strokeWidth={2}
              />
            </AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}
