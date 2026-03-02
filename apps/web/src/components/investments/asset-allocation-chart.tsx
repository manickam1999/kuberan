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
  const { chartConfig, chartData } = useMemo(() => {
    // Filter to types with count > 0 and sort alphabetically for deterministic color assignment
    const activeTypes = (Object.keys(holdingsByType) as AssetType[])
      .filter((type) => holdingsByType[type].count > 0)
      .sort();

    if (activeTypes.length === 0) {
      return { chartConfig: {} as ChartConfig, chartData: [] };
    }

    // Build chart config - assign theme colors based on sorted order
    const config: ChartConfig = {
      value: { label: "Value" },
    };
    activeTypes.forEach((type, index) => {
      config[type] = {
        label: ASSET_TYPE_LABELS[type] ?? type,
        color: CHART_COLORS[index % CHART_COLORS.length],
      };
    });

    // Build chart data (convert cents to dollars)
    const cData = activeTypes.map((type) => ({
      name: type,
      value: holdingsByType[type].value / 100,
      fill: `var(--color-${type})`,
      assetType: type,
    }));

    return { chartConfig: config, chartData: cData };
  }, [holdingsByType]);

  if (chartData.length === 0) {
    return (
      <Card className="flex flex-col">
        <CardHeader className="items-center pb-0">
          <CardTitle>Asset Allocation</CardTitle>
          <CardDescription>Portfolio breakdown by asset type</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-1 flex-col items-center justify-center py-10 text-muted-foreground">
          <p className="text-sm">No holdings</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="flex flex-col">
      <CardHeader className="items-center pb-0">
        <CardTitle>Asset Allocation</CardTitle>
        <CardDescription>Portfolio breakdown by asset type</CardDescription>
      </CardHeader>
      <CardContent className="flex-1 pb-0">
        <ChartContainer
          config={chartConfig}
          className="mx-auto aspect-square w-full max-w-[280px] max-h-[280px] md:max-w-[250px] md:max-h-[250px]"
        >
          <PieChart>
            <ChartTooltip
              cursor={false}
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null;
                const item = payload[0];
                const value = item.value as number;
                // Access assetType from payload and lookup the label
                const assetType = item.payload.assetType as AssetType;
                const pct =
                  totalValue > 0
                    ? ((value * 100) / (totalValue / 100)).toFixed(1)
                    : "0";
                return (
                  <div className="border-border/50 bg-background rounded-lg border px-3 py-2 text-xs shadow-xl">
                    <div className="flex items-center gap-2">
                      <div
                        className="h-2.5 w-2.5 shrink-0 rounded-[2px]"
                        style={{ backgroundColor: item.payload.fill }}
                      />
                      <span className="font-medium">{ASSET_TYPE_LABELS[assetType] ?? assetType}</span>
                    </div>
                    <div className="text-muted-foreground mt-1">
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
              innerRadius={70}
              strokeWidth={5}
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
                          className="fill-foreground text-sm md:text-base font-bold"
                        >
                          {formatCurrency(totalValue)}
                        </tspan>
                        <tspan
                          x={viewBox.cx}
                          y={(viewBox.cy || 0) + 18}
                          className="fill-muted-foreground text-[10px] md:text-xs"
                        >
                          Total Value
                        </tspan>
                      </text>
                    );
                  }
                }}
              />
            </Pie>
          </PieChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
