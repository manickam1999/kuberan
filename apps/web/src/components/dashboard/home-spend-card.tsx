"use client";

import { useMemo } from "react";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import { HomeCard } from "@/components/dashboard/home-bits";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import { Skeleton } from "@/components/ui/skeleton";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useAuth } from "@/hooks/use-auth";
import { useDailySpending, useMonthlySummary } from "@/hooks/use-transactions";
import { formatMoney } from "@/lib/format";
import { cumulative } from "@/lib/home";

const CHART_MARGIN = { top: 6, right: 10, bottom: 0, left: 10 } as const;

function monthKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

/** "Spent this month": the month's spend, a running line, and last month for scale. */
export function HomeSpendCard() {
  const active = useActiveMonth();
  const baseCurrency = useBaseCurrency();
  const { hideBalances } = useAuth();
  const summary = useMonthlySummary(12);
  const daily = useDailySpending(active.fromDate, active.toDate);

  const { row, previous } = useMemo(() => {
    const rows = summary.data ?? [];
    const key = monthKey(active.fromDate);
    const i = rows.findIndex((r) => r.month === key);
    return {
      row: i >= 0 ? rows[i] : undefined,
      previous: i > 0 ? rows[i - 1] : undefined,
    };
  }, [summary.data, active.fromDate]);

  const series = useMemo(
    () =>
      cumulative(daily.data ?? []).map((d) => ({
        label: new Date(`${d.date.split("T")[0]}T00:00:00`).toLocaleDateString(
          "en-US",
          { month: "short", day: "numeric" }
        ),
        value: d.total / 100,
      })),
    [daily.data]
  );

  const title = active.isCurrentMonth
    ? "Spent this month"
    : `Spent in ${active.label.split(" ")[0]}`;
  const currency = row?.currency ?? baseCurrency;
  const spent = row?.expenses ?? 0;
  const diff =
    previous && previous.expenses > 0 ? spent - previous.expenses : null;
  const previousName = previous
    ? new Date(`${previous.month}-01T00:00:00`).toLocaleDateString("en-US", {
        month: "long",
      })
    : "";

  if (summary.isLoading || active.isLoading) {
    return (
      <HomeCard title={title} href="/analytics" linkLabel="Insights">
        <Skeleton className="h-9 w-40" />
        <Skeleton className="h-[90px] w-full" />
        <Skeleton className="h-4 w-48" />
      </HomeCard>
    );
  }

  if (summary.error) {
    return (
      <HomeCard title={title} href="/analytics" linkLabel="Insights">
        <p className="py-6 text-center text-sm text-destructive">
          Could not load this month&apos;s spending.
        </p>
      </HomeCard>
    );
  }

  if (spent === 0) {
    return (
      <HomeCard title={title} href="/analytics" linkLabel="Insights">
        <p className="py-6 text-center text-sm text-muted-foreground">
          No spending recorded in {active.label} yet.
        </p>
      </HomeCard>
    );
  }

  return (
    <HomeCard title={title} href="/analytics" linkLabel="Insights">
      <Money amount={spent} currency={currency} size="lg" className="block" />
      {series.length > 1 && (
        <ThemedAreaChart
          data={series}
          seriesLabel="Spent so far"
          className="h-[90px] w-full"
          ditherColor="green"
          cleanColor="var(--chart-1)"
          margin={CHART_MARGIN}
          hideXAxis
          valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
        />
      )}
      <p className="text-xs text-muted-foreground">
        {active.label}
        {diff !== null && (
          <>
            {" · "}
            <Money
              amount={Math.abs(diff)}
              currency={currency}
              className="text-foreground"
            />{" "}
            {diff <= 0 ? "less" : "more"} than {previousName}
          </>
        )}
      </p>
      <FxNotice fxMissing={row?.fx_missing} variant="inline" />
    </HomeCard>
  );
}
