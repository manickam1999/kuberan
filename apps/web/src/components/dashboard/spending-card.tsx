"use client";

import { useMemo } from "react";
import { ThemedDonut, type DonutSlice } from "@/components/charts/themed-donut";
import { DitherFill } from "@/components/charts/dither-fill";
import { ditherColorAt, ditherFill } from "@/lib/dither";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useSpendingByCategory } from "@/hooks/use-transactions";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { formatCurrency } from "@/lib/format";

const TOP_N = 5;
const OTHERS_CLEAN_COLOR = "#8b8b95";

export function SpendingCard() {
  const {
    fromDate,
    toDate,
    label: monthLabel,
    isCurrentMonth,
  } = useActiveMonth();
  const { data, isLoading } = useSpendingByCategory(fromDate, toDate);
  const { chartTheme } = useChartTheme();

  const { slices, bars } = useMemo(() => {
    if (!data?.items)
      return {
        slices: [] as DonutSlice[],
        bars: [] as (DonutSlice & { pct: number })[],
      };

    const sorted = [...data.items];
    const top = sorted.slice(0, TOP_N);
    const rest = sorted.slice(TOP_N);
    const items: DonutSlice[] = top.map((item, index) => ({
      name: item.category_name,
      value: item.total,
      cleanColor: item.category_color,
      ditherColor: ditherColorAt(index),
    }));
    if (rest.length > 0) {
      items.push({
        name: "Others",
        value: rest.reduce((s, x) => s + x.total, 0),
        cleanColor: OTHERS_CLEAN_COLOR,
        ditherColor: "grey",
      });
    }

    const max = Math.max(1, ...items.map((i) => i.value));
    const barList = items.map((item) => ({ ...item, pct: item.value / max }));

    return { slices: items, bars: barList };
  }, [data]);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Spending</CardTitle>
        </CardHeader>
        <CardContent className="flex gap-6">
          <Skeleton className="size-[200px] shrink-0 rounded-full" />
          <div className="flex-1 space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        </CardContent>
      </Card>
    );
  }

  if (!data || data.items.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Spending</CardTitle>
          <CardDescription>{monthLabel}</CardDescription>
        </CardHeader>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          No expenses recorded
        </CardContent>
      </Card>
    );
  }

  const swatch = (slice: DonutSlice) =>
    chartTheme === "dither" ? ditherFill(slice.ditherColor) : slice.cleanColor;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <div>
            <CardTitle className="text-base">Spending</CardTitle>
            <CardDescription>
              {monthLabel}
              {!isCurrentMonth && " · latest activity"}
            </CardDescription>
          </div>
          <span className="text-xs text-muted-foreground">
            {data.items.length} categories
          </span>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col items-center gap-6 sm:flex-row sm:items-center">
        <ThemedDonut
          data={slices}
          size={200}
          valueFormatter={(value) => formatCurrency(value)}
          totalForPct={data.total_spent}
          center={
            <>
              <span className="text-sm font-semibold tabular-nums">
                {formatCurrency(data.total_spent)}
              </span>
              <span className="text-[10px] text-muted-foreground">spent</span>
            </>
          }
        />

        <ul className="w-full flex-1 space-y-2.5">
          {bars.map((b) => (
            <li key={b.name} className="space-y-1">
              <div className="flex items-center justify-between gap-3 text-sm">
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className="size-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: swatch(b) }}
                  />
                  <span className="truncate">{b.name}</span>
                </span>
                <span className="money shrink-0 font-medium">
                  {formatCurrency(b.value)}
                </span>
              </div>
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className="relative h-full overflow-hidden rounded-full"
                  style={{
                    width: `${Math.max(b.pct * 100, 2)}%`,
                    backgroundColor:
                      chartTheme === "dither" ? undefined : b.cleanColor,
                  }}
                >
                  {chartTheme === "dither" && (
                    <DitherFill color={b.ditherColor} />
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
