"use client";

import Link from "next/link";
import { useAuth } from "@/hooks/use-auth";
import { useAnalyticsPeriod } from "@/hooks/use-analytics-period";
import { useCashflow, useTopExpenses } from "@/hooks/use-transactions";
import { useBaseCurrency } from "@/hooks/use-profile";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import { FxNotice } from "@/components/ui/fx-notice";
import { TopExpensesList } from "@/components/analytics/top-expenses-list";
import { DailySpendingCalendar } from "@/components/analytics/daily-spending-calendar";
import { CashflowSankeyCard } from "@/components/analytics/cashflow-sankey-card";
import { CategoryBreakdownCard } from "@/components/analytics/category-breakdown-card";
import { MonthlyTrendCard } from "@/components/analytics/monthly-trend-card";
import { PeriodSelector } from "@/components/analytics/period-selector";
import { formatDate } from "@/lib/format";
import { savingsRate, transactionsHref } from "@/lib/insights";

export default function InsightsPage() {
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const period = useAnalyticsPeriod();
  const { fromDate, toDate, label, isActiveMonthDefault } = period;

  const cashflow = useCashflow(fromDate, toDate);
  const top = useTopExpenses(fromDate, toDate, 1);

  const flow = cashflow.data;
  const currency = flow?.currency ?? baseCurrency;
  const topItem = top.data?.items[0];
  const saved = flow ? flow.total_income - flow.total_expenses : 0;
  const rate = flow ? savingsRate(flow.total_income, flow.total_expenses) : null;
  const incomeSources = (flow?.income ?? [])
    .filter((i) => i.total > 0)
    .slice(0, 2)
    .map((i) => i.category_name)
    .join(" + ");
  const statsLoading = cashflow.isLoading || top.isLoading;

  const items: StatStripItem[] = flow
    ? [
        {
          label: "Spent",
          value: (
            <Link
              href={transactionsHref({ from: fromDate, to: toDate, type: "expense" })}
              className="focus-visible:ring-ring/50 rounded-sm outline-none hover:underline focus-visible:ring-[3px]"
            >
              <Money amount={flow.total_expenses} currency={currency} sign="never" />
            </Link>
          ),
          sub: top.data
            ? `${top.data.total_count} ${top.data.total_count === 1 ? "expense" : "expenses"}`
            : undefined,
        },
        {
          label: "Earned",
          value: (
            <Link
              href={transactionsHref({ from: fromDate, to: toDate, type: "income" })}
              className="focus-visible:ring-ring/50 rounded-sm outline-none hover:underline focus-visible:ring-[3px]"
            >
              <Money amount={flow.total_income} currency={currency} sign="never" />
            </Link>
          ),
          tone: flow.total_income > 0 ? "pos" : "default",
          sub: incomeSources || undefined,
        },
        {
          label: "Saved",
          value: rate === null ? "—" : `${rate}%`,
          tone: rate === null ? "default" : rate >= 0 ? "pos" : "neg",
          sub:
            rate === null ? undefined : (
              <Money amount={saved} currency={currency} sign="auto" />
            ),
        },
        {
          label: "Biggest",
          value: topItem ? (
            topItem.fx_missing ? (
              "—"
            ) : (
              <Money amount={topItem.amount_base} currency={currency} sign="never" />
            )
          ) : (
            "—"
          ),
          sub: topItem
            ? `${topItem.category_name} · ${formatDate(topItem.date)}`
            : undefined,
        },
      ]
    : [];

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-muted-foreground text-xs">Where your money went</p>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Insights
          </h1>
        </div>
        <PeriodSelector period={period} />
      </div>

      {statsLoading ? (
        <Skeleton className="h-[88px] w-full rounded-card" />
      ) : cashflow.isError ? (
        <div
          role="alert"
          className="rounded-card border border-dashed p-6 text-center text-sm"
        >
          <p className="font-medium">Couldn&apos;t load your insights.</p>
          <button
            type="button"
            className="text-primary mt-1 underline underline-offset-2"
            onClick={() => void cashflow.refetch()}
          >
            Try again
          </button>
        </div>
      ) : (
        <StatStrip items={items} />
      )}

      <FxNotice
        fxMissing={flow?.fx_missing}
        unconverted={flow?.unconverted}
        variant="inline"
      />

      <CashflowSankeyCard
        fromDate={fromDate}
        toDate={toDate}
        label={label}
        isActiveMonthDefault={isActiveMonthDefault}
      />

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
        <DailySpendingCalendar
          fromDate={fromDate}
          toDate={toDate}
          hideBalances={hideBalances}
        />
        <CategoryBreakdownCard fromDate={fromDate} toDate={toDate} />
      </div>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
        <TopExpensesList fromDate={fromDate} toDate={toDate} />
        <MonthlyTrendCard />
      </div>
    </div>
  );
}
