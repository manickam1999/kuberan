"use client";

import { DollarSign, Receipt, Calculator, TrendingDown } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useAnalyticsPeriod } from "@/hooks/use-analytics-period";
import {
  useSpendingByCategory,
  useTopExpenses,
} from "@/hooks/use-transactions";
import { StatCard } from "@/components/ui/stat-card";
import { SpendingCard } from "@/components/dashboard/spending-card";
import { CashflowCard } from "@/components/dashboard/cashflow-card";
import { TopExpensesList } from "@/components/analytics/top-expenses-list";
import { DailySpendingCalendar } from "@/components/analytics/daily-spending-calendar";
import { CashflowSankeyCard } from "@/components/analytics/cashflow-sankey-card";
import { PeriodSelector } from "@/components/analytics/period-selector";
import { Skeleton } from "@/components/ui/skeleton";
import { formatMoney } from "@/lib/format";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";

export default function AnalyticsPage() {
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const period = useAnalyticsPeriod();
  const { fromDate, toDate, label, isActiveMonthDefault } = period;

  const { data: spending, isLoading: spendingLoading } = useSpendingByCategory(
    fromDate,
    toDate
  );
  const { data: topExpense, isLoading: topExpenseLoading } = useTopExpenses(
    fromDate,
    toDate,
    1
  );

  // Both totals are in the response's currency (the user's base).
  const spendCurrency = spending?.currency ?? baseCurrency;
  const topCurrency = topExpense?.currency ?? baseCurrency;
  const totalSpent = spending?.total_spent ?? 0;
  // Count only expenses with a non-zero effective amount (personal share).
  const totalItems = topExpense?.total_count ?? 0;
  const avgAmount = totalItems > 0 ? Math.round(totalSpent / totalItems) : 0;
  const topItem = topExpense?.items[0];
  // amount_base is 0 when the expense had no rate, so show a dash instead.
  const largestText = !topItem
    ? formatMoney(0, topCurrency, hideBalances)
    : topItem.fx_missing
      ? "—"
      : formatMoney(topItem.amount_base, topCurrency, hideBalances);

  const statsLoading = spendingLoading || topExpenseLoading;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">
          Expense Analytics
        </h1>
        <PeriodSelector period={period} />
      </div>

      {statsLoading ? (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[112px] w-full" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            label="Total spent"
            value={formatMoney(totalSpent, spendCurrency, hideBalances)}
            icon={DollarSign}
            tone="negative"
          />
          <StatCard
            label="Transactions"
            value={String(totalItems)}
            icon={Receipt}
          />
          <StatCard
            label="Average expense"
            value={formatMoney(avgAmount, spendCurrency, hideBalances)}
            icon={Calculator}
          />
          <StatCard
            label="Largest expense"
            value={largestText}
            icon={TrendingDown}
            tone="negative"
          />
        </div>
      )}

      <FxNotice
        fxMissing={spending?.fx_missing}
        unconverted={spending?.unconverted}
        variant="inline"
      />

      <CashflowSankeyCard
        fromDate={fromDate}
        toDate={toDate}
        label={label}
        isActiveMonthDefault={isActiveMonthDefault}
      />

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-3">
        <div className="min-w-0 space-y-4 lg:col-span-2">
          <TopExpensesList
            fromDate={fromDate}
            toDate={toDate}
            hideBalances={hideBalances}
          />
          <CashflowCard
            title="Monthly trend"
            label="Last 6 months"
            isActiveMonthDefault={false}
          />
        </div>

        <div className="min-w-0 space-y-4">
          <DailySpendingCalendar
            fromDate={fromDate}
            toDate={toDate}
            hideBalances={hideBalances}
          />
          <SpendingCard
            fromDate={fromDate}
            toDate={toDate}
            label={label}
            isActiveMonthDefault={isActiveMonthDefault}
          />
        </div>
      </div>
    </div>
  );
}
