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
import { useTypical } from "@/hooks/use-typical";
import { formatMoney, toLocalDateString } from "@/lib/format";
import { cumulative } from "@/lib/home";
import { hasTypical, paceSeries, typicalSpendFor } from "@/lib/typical";

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
  const typicalQuery = useTypical(6);
  const typical = hasTypical(typicalQuery.data) ? typicalQuery.data : undefined;

  const { row, previous } = useMemo(() => {
    const rows = summary.data ?? [];
    const key = monthKey(active.fromDate);
    const i = rows.findIndex((r) => r.month === key);
    return {
      row: i >= 0 ? rows[i] : undefined,
      previous: i > 0 ? rows[i - 1] : undefined,
    };
  }, [summary.data, active.fromDate]);

  const { series, typicalLine } = useMemo(() => {
    const cum = cumulative(daily.data ?? []);
    // The running total of an ongoing month stops today; later days are empty.
    const throughDay = active.isCurrentMonth
      ? new Date().getDate()
      : cum.length;
    const pace = paceSeries(
      cum.map((d) => d.total),
      typical?.daily_cumulative ?? [],
      throughDay
    );
    return {
      series: pace.actual.map((total, i) => ({
        label: new Date(
          `${cum[i].date.split("T")[0]}T00:00:00`
        ).toLocaleDateString("en-US", { month: "short", day: "numeric" }),
        value: total / 100,
      })),
      typicalLine: typical ? pace.typical.map((v) => v / 100) : undefined,
    };
  }, [daily.data, typical, active.isCurrentMonth]);

  const title = active.isCurrentMonth
    ? "Spent this month"
    : `Spent in ${active.label.split(" ")[0]}`;
  const currency = row?.currency ?? baseCurrency;
  const spent = row?.expenses ?? 0;
  const diff =
    previous && previous.expenses > 0 ? spent - previous.expenses : null;
  const typicalSoFar = typicalSpendFor(
    typical,
    toLocalDateString(new Date(active.fromDate)),
    new Date()
  );
  const paceDiff = typicalSoFar !== null ? spent - typicalSoFar : null;
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
          reference={typicalLine}
          valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
        />
      )}
      {paceDiff !== null && typicalLine && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1.5">
            <i className="inline-block size-2 rounded-[2px] bg-chart-1" />
            {active.label.split(" ")[0]}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <i className="inline-block h-0 w-3 border-t border-dashed border-muted-foreground" />
            Typical
          </span>
          <span
            className={
              paceDiff === 0
                ? "ml-auto"
                : paceDiff < 0
                  ? "ml-auto text-positive"
                  : "ml-auto text-negative"
            }
          >
            {paceDiff === 0 ? (
              "On typical pace"
            ) : (
              <>
                <Money amount={Math.abs(paceDiff)} currency={currency} />{" "}
                {paceDiff < 0 ? "under" : "over"}
              </>
            )}
          </span>
        </div>
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
