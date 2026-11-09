"use client";

import { useMemo } from "react";
import type { DonutSlice } from "@/components/charts/themed-donut";
import { LazyThemedDonut as ThemedDonut } from "@/components/charts/lazy";
import { ditherColorAt, ditherFill } from "@/lib/dither";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useAllInvestments } from "@/hooks/use-investments";
import { formatMoney } from "@/lib/format";

const CLEAN_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--chart-6)",
];

const PAGE_SIZE = 100;

/**
 * Donut of open holdings' value in the base currency, grouped by the
 * securities' price currency. Holdings without an FX rate are left out.
 */
export function CurrencyExposureChart() {
  const { chartTheme } = useChartTheme();
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const { data, isLoading } = useAllInvestments({
    page: 1,
    page_size: PAGE_SIZE,
    status: "open",
  });

  const { rows, total } = useMemo(() => {
    const byCurrency = new Map<string, number>();
    for (const inv of data?.data ?? []) {
      if (inv.fx_missing || inv.quantity <= 0) continue;
      const value = Math.round(inv.quantity * inv.current_price); // base
      byCurrency.set(
        inv.price_currency,
        (byCurrency.get(inv.price_currency) ?? 0) + value
      );
    }
    const sum = [...byCurrency.values()].reduce((a, b) => a + b, 0);
    const list = [...byCurrency.entries()]
      .filter(([, value]) => value > 0)
      .sort((a, b) => b[1] - a[1])
      .map(([currency, value], index) => ({
        name: currency,
        value,
        cleanColor: CLEAN_COLORS[index % CLEAN_COLORS.length],
        ditherColor: ditherColorAt(index),
        pct: sum > 0 ? (value / sum) * 100 : 0,
      }));
    return { rows: list, total: sum };
  }, [data?.data]);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-36" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[200px] w-full" />
        </CardContent>
      </Card>
    );
  }

  if (rows.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Currency exposure</CardTitle>
        </CardHeader>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          No holdings
        </CardContent>
      </Card>
    );
  }

  const swatch = (slice: DonutSlice) =>
    chartTheme === "dither" ? ditherFill(slice.ditherColor) : slice.cleanColor;
  const truncated = (data?.total_items ?? 0) > PAGE_SIZE;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Currency exposure</CardTitle>
        <CardDescription>
          Value in {baseCurrency} by security currency
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col items-center gap-5">
        <div className="flex items-center justify-center py-2">
          <ThemedDonut
            data={rows.map(({ name, value, cleanColor, ditherColor }) => ({
              name,
              value,
              cleanColor,
              ditherColor,
            }))}
            size={200}
            valueFormatter={(value) =>
              formatMoney(value, baseCurrency, hideBalances)
            }
            totalForPct={total}
            center={
              <>
                <span className="text-sm font-semibold tabular-nums">
                  {rows.length}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {rows.length === 1 ? "currency" : "currencies"}
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
              <span className="min-w-0 flex-1 truncate">{r.name}</span>
              <span className="money shrink-0 font-medium">
                {formatMoney(r.value, baseCurrency, hideBalances)}
              </span>
              <span className="w-10 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                {r.pct.toFixed(0)}%
              </span>
            </li>
          ))}
        </ul>
        {truncated && (
          <p className="text-xs text-muted-foreground">
            Based on the first {PAGE_SIZE} holdings.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
