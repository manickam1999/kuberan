"use client";

import { useMemo } from "react";
import { Label, Pie, PieChart } from "recharts";
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
import { useSpendingByCategory } from "@/hooks/use-transactions";
import { useActiveMonth } from "@/hooks/use-active-month";
import { formatCurrency } from "@/lib/format";

const TOP_N = 5;
const OTHERS_COLOR = "#8b8b95";

export function SpendingCard() {
  const {
    fromDate,
    toDate,
    label: monthLabel,
    isCurrentMonth,
  } = useActiveMonth();
  const { data, isLoading } = useSpendingByCategory(fromDate, toDate);

  const { chartConfig, chartData, bars } = useMemo(() => {
    if (!data?.items)
      return {
        chartConfig: {} as ChartConfig,
        chartData: [],
        bars: [] as { name: string; value: number; fill: string; pct: number }[],
      };

    const sorted = [...data.items];
    const top = sorted.slice(0, TOP_N);
    const rest = sorted.slice(TOP_N);
    const items = [...top];
    if (rest.length > 0) {
      items.push({
        category_id: null,
        category_name: "Others",
        category_color: OTHERS_COLOR,
        category_icon: "",
        total: rest.reduce((s, x) => s + x.total, 0),
      });
    }

    const config: ChartConfig = { total: { label: "Spending" } };
    const cData = items.map((item) => ({
      name: item.category_name,
      value: item.total,
      fill: item.category_color,
    }));
    const max = Math.max(1, ...items.map((i) => i.total));
    const barList = items.map((item) => ({
      name: item.category_name,
      value: item.total,
      fill: item.category_color,
      pct: item.total / max,
    }));

    return { chartConfig: config, chartData: cData, bars: barList };
  }, [data]);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Spending</CardTitle>
        </CardHeader>
        <CardContent className="flex gap-6">
          <Skeleton className="size-[150px] shrink-0 rounded-full" />
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
        <ChartContainer
          config={chartConfig}
          className="aspect-square size-[150px] shrink-0"
        >
          <PieChart>
            <ChartTooltip
              cursor={false}
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null;
                const item = payload[0];
                const value = item.value as number;
                const pct =
                  data.total_spent > 0
                    ? ((value / data.total_spent) * 100).toFixed(1)
                    : "0";
                return (
                  <div className="rounded-lg border border-border/50 bg-popover px-3 py-2 text-xs shadow-xl">
                    <div className="flex items-center gap-2">
                      <span
                        className="size-2.5 rounded-[2px]"
                        style={{ backgroundColor: item.payload.fill }}
                      />
                      <span className="font-medium">{item.name}</span>
                    </div>
                    <div className="mt-1 text-muted-foreground">
                      {formatCurrency(value)} ({pct}%)
                    </div>
                  </div>
                );
              }}
            />
            <Pie
              data={chartData}
              dataKey="value"
              nameKey="name"
              innerRadius={52}
              strokeWidth={4}
            >
              <Label
                content={({ viewBox }) => {
                  if (viewBox && "cx" in viewBox && "cy" in viewBox) {
                    return (
                      <text
                        x={viewBox.cx}
                        y={viewBox.cy}
                        textAnchor="middle"
                        dominantBaseline="middle"
                      >
                        <tspan
                          x={viewBox.cx}
                          y={viewBox.cy}
                          className="fill-foreground text-sm font-semibold"
                        >
                          {formatCurrency(data.total_spent)}
                        </tspan>
                        <tspan
                          x={viewBox.cx}
                          y={(viewBox.cy || 0) + 16}
                          className="fill-muted-foreground text-[10px]"
                        >
                          spent
                        </tspan>
                      </text>
                    );
                  }
                }}
              />
            </Pie>
          </PieChart>
        </ChartContainer>

        <ul className="w-full flex-1 space-y-2.5">
          {bars.map((b) => (
            <li key={b.name} className="space-y-1">
              <div className="flex items-center justify-between gap-3 text-sm">
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className="size-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: b.fill }}
                  />
                  <span className="truncate">{b.name}</span>
                </span>
                <span className="money shrink-0 font-medium">
                  {formatCurrency(b.value)}
                </span>
              </div>
              <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full"
                  style={{
                    width: `${Math.max(b.pct * 100, 2)}%`,
                    backgroundColor: b.fill,
                  }}
                />
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
