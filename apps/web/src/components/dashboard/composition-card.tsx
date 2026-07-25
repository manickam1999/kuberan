"use client";

import { ArrowUpRight, ArrowDownRight } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useAccounts } from "@/hooks/use-accounts";
import { usePortfolio } from "@/hooks/use-investments";
import { Skeleton } from "@/components/ui/skeleton";
import { DitherFill } from "@/components/charts/dither-fill";
import { ditherFill } from "@/lib/dither";
import type { DitherColor } from "@/components/dither-kit/palette";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { useAuth } from "@/hooks/use-auth";
import { formatCurrency, formatPercentage } from "@/lib/format";
import type { Account } from "@/types/models";

interface Segment {
  label: string;
  value: number;
  color: string;
  ditherColor: DitherColor;
  pct: number;
  delta?: { text: string; up: boolean };
}

export function CompositionCard() {
  const { data: accountsData, isLoading } = useAccounts();
  const { data: portfolio } = usePortfolio();
  const { chartTheme } = useChartTheme();
  const { hideBalances } = useAuth();

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Composition</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-3 w-full rounded-full" />
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-5 w-full" />
          ))}
        </CardContent>
      </Card>
    );
  }

  const accounts = accountsData?.data ?? [];
  const sumByType = (type: Account["type"]) =>
    accounts.filter((a) => a.type === type).reduce((s, a) => s + a.balance, 0);

  const cash = sumByType("cash");
  const investmentAccounts = sumByType("investment");
  const investments = portfolio?.total_value ?? investmentAccounts;
  const debt = sumByType("credit_card") + sumByType("debt");
  const assets = cash + investments;
  const netWorth = assets - debt;
  const gain = portfolio?.total_gain_loss ?? 0;

  const assetSegments: Segment[] = [
    {
      label: "Cash",
      value: cash,
      color: "var(--chart-1)",
      ditherColor: "green",
      pct: assets > 0 ? (cash / assets) * 100 : 0,
    },
    {
      label: "Investments",
      value: investments,
      color: "var(--chart-3)",
      ditherColor: "purple",
      pct: assets > 0 ? (investments / assets) * 100 : 0,
      delta: portfolio
        ? {
            text: `${gain >= 0 ? "+" : ""}${formatPercentage(portfolio.gain_loss_pct)}`,
            up: gain >= 0,
          }
        : undefined,
    },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Composition</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* 100% stacked allocation bar */}
        <div className="flex h-2.5 w-full gap-0.5 overflow-hidden rounded-full">
          {assetSegments.map((s) => (
            <div
              key={s.label}
              style={{
                width: `${Math.max(s.pct, 2)}%`,
                backgroundColor:
                  chartTheme === "dither" ? undefined : s.color,
              }}
              className="relative h-full overflow-hidden first:rounded-l-full last:rounded-r-full"
            >
              {chartTheme === "dither" && <DitherFill color={s.ditherColor} />}
            </div>
          ))}
        </div>

        <div className="space-y-3">
          {assetSegments.map((s) => (
            <div
              key={s.label}
              className="flex items-center justify-between gap-3"
            >
              <span className="flex min-w-0 items-center gap-2">
                <span
                  className="size-2.5 shrink-0 rounded-full"
                  style={{
                    backgroundColor:
                      chartTheme === "dither"
                        ? ditherFill(s.ditherColor)
                        : s.color,
                  }}
                />
                <span className="truncate text-sm text-muted-foreground">
                  {s.label}
                </span>
                {s.delta && (
                  <span
                    className={`inline-flex shrink-0 items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[11px] font-medium ${
                      s.delta.up
                        ? "bg-positive-muted text-positive"
                        : "bg-negative-muted text-negative"
                    }`}
                  >
                    {s.delta.up ? (
                      <ArrowUpRight className="size-3" />
                    ) : (
                      <ArrowDownRight className="size-3" />
                    )}
                    {s.delta.text}
                  </span>
                )}
              </span>
              <span className="money shrink-0 text-sm font-medium">
                {formatCurrency(s.value, undefined, hideBalances)}
              </span>
            </div>
          ))}

          {debt > 0 && (
            <div className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-2">
                <span
                  className="size-2.5 shrink-0 rounded-full bg-negative"
                  style={
                    chartTheme === "dither"
                      ? { backgroundColor: ditherFill("red") }
                      : undefined
                  }
                />
                <span className="text-sm text-muted-foreground">Debt</span>
              </span>
              <span className="money shrink-0 text-sm font-medium text-negative">
                −{formatCurrency(debt, undefined, hideBalances)}
              </span>
            </div>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-border/60 pt-3">
          <span className="text-sm font-medium">Net worth</span>
          <span className="money text-base font-semibold">
            {formatCurrency(netWorth, undefined, hideBalances)}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
