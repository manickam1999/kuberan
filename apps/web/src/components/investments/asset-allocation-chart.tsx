"use client";

import { useMemo } from "react";
import { ThemedDonut, type DonutSlice } from "@/components/charts/themed-donut";
import { ditherColorAt, ditherFill } from "@/lib/dither";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useChartTheme } from "@/providers/chart-theme-provider";
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

// Clean-theme slice colours (the shared chart palette CSS vars).
const CLEAN_COLORS = [
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
  const { chartTheme } = useChartTheme();

  const { slices, rows } = useMemo(() => {
    const activeTypes = (Object.keys(holdingsByType) as AssetType[])
      .filter((type) => holdingsByType[type].count > 0)
      .sort((a, b) => holdingsByType[b].value - holdingsByType[a].value);

    const sliceList: (DonutSlice & { count: number; pct: number })[] =
      activeTypes.map((type, index) => ({
        name: ASSET_TYPE_LABELS[type] ?? type,
        value: holdingsByType[type].value,
        cleanColor: CLEAN_COLORS[index % CLEAN_COLORS.length],
        ditherColor: ditherColorAt(index),
        count: holdingsByType[type].count,
        pct:
          totalValue > 0
            ? (holdingsByType[type].value / totalValue) * 100
            : 0,
      }));

    return {
      slices: sliceList.map(({ name, value, cleanColor, ditherColor }) => ({
        name,
        value,
        cleanColor,
        ditherColor,
      })),
      rows: sliceList,
    };
  }, [holdingsByType, totalValue]);

  if (rows.length === 0) {
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

  const swatch = (slice: DonutSlice) =>
    chartTheme === "dither" ? ditherFill(slice.ditherColor) : slice.cleanColor;

  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="text-base">Asset allocation</CardTitle>
        <CardDescription>By asset class</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col items-center gap-5">
        <div className="flex flex-1 items-center justify-center py-2">
          <ThemedDonut
            data={slices}
            size={160}
            valueFormatter={(value) => formatCurrency(value)}
            totalForPct={totalValue}
            center={
              <>
                <span className="text-sm font-semibold tabular-nums">
                  {rows.length}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  classes
                </span>
              </>
            }
          />
        </div>

        <ul className="w-full space-y-2.5">
          {rows.map((r) => (
            <li key={r.name} className="flex items-center gap-3 text-sm">
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: swatch(r) }}
              />
              <span className="min-w-0 flex-1 truncate">
                {r.name}
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
