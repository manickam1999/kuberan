"use client";

import { useMemo, useState } from "react";
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
import { usePortfolioSnapshots } from "@/hooks/use-portfolio-snapshots";
import { formatCurrency, formatDate } from "@/lib/format";

const PERIOD_OPTIONS = [
  { value: "1M", label: "1M", months: 1 },
  { value: "3M", label: "3M", months: 3 },
  { value: "6M", label: "6M", months: 6 },
  { value: "1Y", label: "1Y", months: 12 },
  { value: "ALL", label: "ALL", months: 120 },
] as const;

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
        label: formatDate(s.recorded_at),
        value: s.investment_value / 100,
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
          <ThemedAreaChart
            data={chartData}
            seriesLabel="Investment Value"
            className="h-[220px] md:h-[280px] w-full"
            ditherColor="green"
            cleanColor="var(--chart-1)"
            valueFormatter={(v) => formatCurrency(v * 100)}
          />
        )}
      </CardContent>
    </Card>
  );
}
