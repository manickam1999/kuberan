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
import { formatCurrency } from "@/lib/format";
import type { AssetType } from "@/types/models";

const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stocks",
  etf: "ETFs",
  bond: "Bonds",
  crypto: "Crypto",
  reit: "REITs",
  commodity: "Commodities",
};

const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--chart-6)",
];

interface AssetAllocationChartProps {
  holdingsByType: Record<AssetType, { value: number; count: number }>;
  totalValue: number;
}

export function AssetAllocationChart({
  holdingsByType,
  totalValue,
}: AssetAllocationChartProps) {
  const { chartConfig, chartData, rows } = useMemo(() => {
    const activeTypes = (Object.keys(holdingsByType) as AssetType[])
      .filter((type) => holdingsByType[type].count > 0)
      .sort((a, b) => holdingsByType[b].value - holdingsByType[a].value);

    if (activeTypes.length === 0) {
      return {
        chartConfig: {} as ChartConfig,
        chartData: [],
        rows: [] as {
          type: AssetType;
          label: string;
          value: number;
          count: number;
          pct: number;
          color: string;
        }[],
      };
    }

    const config: ChartConfig = { value: { label: "Value" } };
    activeTypes.forEach((type, index) => {
      config[type] = {
        label: ASSET_TYPE_LABELS[type] ?? type,
        color: CHART_COLORS[index % CHART_COLORS.length],
      };
    });

    const cData = activeTypes.map((type) => ({
      name: type,
      value: holdingsByType[type].value / 100,
      fill: `var(--color-${type})`,
      assetType: type,
    }));

    const rowList = activeTypes.map((type, index) => ({
      type,
      label: ASSET_TYPE_LABELS[type] ?? type,
      value: holdingsByType[type].value,
      count: holdingsByType[type].count,
      pct:
        totalValue > 0 ? (holdingsByType[type].value / totalValue) * 100 : 0,
      color: CHART_COLORS[index % CHART_COLORS.length],
    }));

    return { chartConfig: config, chartData: cData, rows: rowList };
  }, [holdingsByType, totalValue]);

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Asset allocation</CardTitle>
        </CardHeader>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          No holdings
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="text-base">Asset allocation</CardTitle>
        <CardDescription>By asset class</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col items-center gap-5">
        <div className="flex flex-1 items-center justify-center py-2">
          <ChartContainer
            config={chartConfig}
            className="aspect-square size-[160px] shrink-0"
          >
          <PieChart>
            <ChartTooltip
              cursor={false}
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null;
                const item = payload[0];
                const value = item.value as number;
                const assetType = item.payload.assetType as AssetType;
                const pct =
                  totalValue > 0
                    ? ((value * 100) / (totalValue / 100)).toFixed(1)
                    : "0";
                return (
                  <div className="rounded-lg border border-border/50 bg-popover px-3 py-2 text-xs shadow-xl">
                    <div className="flex items-center gap-2">
                      <span
                        className="size-2.5 rounded-[2px]"
                        style={{ backgroundColor: item.payload.fill }}
                      />
                      <span className="font-medium">
                        {ASSET_TYPE_LABELS[assetType] ?? assetType}
                      </span>
                    </div>
                    <div className="mt-1 text-muted-foreground">
                      {formatCurrency(value * 100)} ({pct}%)
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
                          {rows.length}
                        </tspan>
                        <tspan
                          x={viewBox.cx}
                          y={(viewBox.cy || 0) + 16}
                          className="fill-muted-foreground text-[10px]"
                        >
                          classes
                        </tspan>
                      </text>
                    );
                  }
                }}
              />
            </Pie>
          </PieChart>
          </ChartContainer>
        </div>

        <ul className="w-full space-y-2.5">
          {rows.map((r) => (
            <li key={r.type} className="flex items-center gap-3 text-sm">
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: r.color }}
              />
              <span className="min-w-0 flex-1 truncate">
                {r.label}
                <span className="ml-1.5 text-xs text-muted-foreground">
                  {r.count}
                </span>
              </span>
              <span className="money shrink-0 font-medium">
                {formatCurrency(r.value)}
              </span>
              <span className="w-10 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                {r.pct.toFixed(0)}%
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
