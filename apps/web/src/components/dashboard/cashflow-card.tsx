"use client";

import { useMemo } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useMonthlySummary } from "@/hooks/use-transactions";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { DitherFill } from "@/components/charts/dither-fill";
import { formatCurrency } from "@/lib/format";

export function CashflowCard() {
  const { data, isLoading } = useMonthlySummary(6);
  const { label: monthLabel, isCurrentMonth } = useActiveMonth();
  const { chartTheme } = useChartTheme();
  const isDither = chartTheme === "dither";

  const { income, expenses, net, trend, savingsRate } = useMemo(() => {
    const rows = data ?? [];
    // Active month = latest row with any activity, else last row.
    const active =
      [...rows].reverse().find((m) => m.income > 0 || m.expenses > 0) ??
      rows[rows.length - 1];
    const income = active?.income ?? 0;
    const expenses = active?.expenses ?? 0;
    const net = income - expenses;
    const savingsRate = income > 0 ? (net / income) * 100 : null;

    const maxNet = Math.max(1, ...rows.map((m) => Math.abs(m.income - m.expenses)));
    const trend = rows.map((m) => {
      const d = m.income - m.expenses;
      const date = new Date(m.month + "-01");
      return {
        label: date.toLocaleDateString("en-US", { month: "short" }),
        full: date.toLocaleDateString("en-US", { month: "long", year: "numeric" }),
        income: m.income,
        expenses: m.expenses,
        net: d,
        height: Math.abs(d) / maxNet,
      };
    });

    return { income, expenses, net, trend, savingsRate };
  }, [data]);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Cash flow</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    );
  }

  const total = income + expenses;
  const incomePct = total > 0 ? (income / total) * 100 : 50;
  const netUp = net >= 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Cash flow</CardTitle>
        <CardDescription>
          {monthLabel}
          {!isCurrentMonth && " · latest activity"}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-3 sm:gap-3">
          <div className="flex items-baseline justify-between gap-2 sm:block">
            <p className="text-xs text-muted-foreground">Income</p>
            <p className="money text-base font-semibold text-positive sm:mt-0.5 sm:text-lg">
              {formatCurrency(income)}
            </p>
          </div>
          <div className="flex items-baseline justify-between gap-2 sm:block">
            <p className="text-xs text-muted-foreground">Expenses</p>
            <p className="money text-base font-semibold text-negative sm:mt-0.5 sm:text-lg">
              {formatCurrency(expenses)}
            </p>
          </div>
          <div className="flex items-baseline justify-between gap-2 sm:block">
            <p className="text-xs text-muted-foreground">Net</p>
            <p
              className={`money text-base font-semibold sm:mt-0.5 sm:text-lg ${
                netUp ? "text-positive" : "text-negative"
              }`}
            >
              {netUp ? "+" : "−"}
              {formatCurrency(Math.abs(net))}
            </p>
          </div>
        </div>

        {/* Income vs expense proportion */}
        <div>
          <div className="flex h-2 w-full overflow-hidden rounded-full bg-muted">
            <div
              className={`relative h-full overflow-hidden ${isDither ? "" : "bg-positive"}`}
              style={{ width: `${incomePct}%` }}
            >
              {isDither && <DitherFill color="green" />}
            </div>
            <div
              className={`relative h-full flex-1 overflow-hidden ${isDither ? "" : "bg-negative"}`}
            >
              {isDither && <DitherFill color="red" />}
            </div>
          </div>
          {savingsRate !== null && (
            <p className="mt-1.5 text-xs text-muted-foreground">
              Savings rate{" "}
              <span
                className={savingsRate >= 0 ? "text-positive" : "text-negative"}
              >
                {savingsRate.toFixed(0)}%
              </span>
            </p>
          )}
        </div>

        {/* 6-month net trend */}
        {trend.length > 1 && (
          <TooltipProvider delayDuration={100}>
            <div className="flex items-end justify-between gap-1.5 pt-1">
              {trend.map((m, i) => (
                <Tooltip key={i}>
                  <TooltipTrigger asChild>
                    <div className="flex flex-1 cursor-default flex-col items-center gap-1.5 rounded-md py-1 transition-colors hover:bg-accent/40">
                      <div className="flex h-10 w-full items-end justify-center">
                        <div
                          className={`relative w-full max-w-6 overflow-hidden rounded-sm ${
                            isDither
                              ? ""
                              : m.net >= 0
                                ? "bg-positive/70"
                                : "bg-negative/70"
                          }`}
                          style={{ height: `${Math.max(m.height * 100, 4)}%` }}
                        >
                          {isDither && (
                            <DitherFill color={m.net >= 0 ? "green" : "red"} />
                          )}
                        </div>
                      </div>
                      <span className="text-[10px] text-muted-foreground">
                        {m.label}
                      </span>
                    </div>
                  </TooltipTrigger>
                  <TooltipContent className="text-xs">
                    <p className="mb-1 font-medium">{m.full}</p>
                    <div className="space-y-0.5">
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">Income</span>
                        <span className="money text-positive">
                          {formatCurrency(m.income)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between gap-4">
                        <span className="text-muted-foreground">Expenses</span>
                        <span className="money text-negative">
                          {formatCurrency(m.expenses)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between gap-4 border-t border-border/40 pt-0.5">
                        <span className="text-muted-foreground">Net</span>
                        <span
                          className={`money ${m.net >= 0 ? "text-positive" : "text-negative"}`}
                        >
                          {m.net >= 0 ? "+" : "−"}
                          {formatCurrency(Math.abs(m.net))}
                        </span>
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              ))}
            </div>
          </TooltipProvider>
        )}
      </CardContent>
    </Card>
  );
}
