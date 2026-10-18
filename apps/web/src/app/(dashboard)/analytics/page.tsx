"use client";

import { DollarSign, Receipt, Calculator, TrendingDown } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useAnalyticsPeriod } from "@/hooks/use-analytics-period";
import {
  useSpendingByCategory,
  useTransactions,
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
import { formatCurrency } from "@/lib/format";

export default function AnalyticsPage() {
  const { hideBalances } = useAuth();
  const period = useAnalyticsPeriod();
  const { fromDate, toDate, label, isActiveMonthDefault } = period;

  const { data: spending, isLoading: spendingLoading } = useSpendingByCategory(
    fromDate,
    toDate
  );
  const { data: expenseCount, isLoading: countLoading } = useTransactions({
    type: "expense",
    from_date: fromDate,
    to_date: toDate,
    page_size: 1,
  });
  const { data: topExpense, isLoading: topExpenseLoading } = useTopExpenses(
    fromDate,
    toDate,
    1
  );

  const totalSpent = spending?.total_spent ?? 0;
  const totalItems = expenseCount?.total_items ?? 0;
  const avgAmount = totalItems > 0 ? Math.round(totalSpent / totalItems) : 0;
  const largestAmount = topExpense?.items[0]?.amount ?? 0;

  const statsLoading = spendingLoading || countLoading || topExpenseLoading;

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
            value={formatCurrency(totalSpent, undefined, hideBalances)}
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
            value={formatCurrency(avgAmount, undefined, hideBalances)}
            icon={Calculator}
          />
          <StatCard
            label="Largest expense"
            value={formatCurrency(largestAmount, undefined, hideBalances)}
            icon={TrendingDown}
            tone="negative"
          />
        </div>
      )}

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
