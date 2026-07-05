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
import { useMonthlySummary } from "@/hooks/use-transactions";
import { useActiveMonth } from "@/hooks/use-active-month";
import { formatCurrency } from "@/lib/format";

export function CashflowCard() {
  const { data, isLoading } = useMonthlySummary(6);
  const { label: monthLabel, isCurrentMonth } = useActiveMonth();

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
      const label = new Date(m.month + "-01").toLocaleDateString("en-US", {
        month: "short",
      });
      return { label, net: d, height: Math.abs(d) / maxNet };
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
        <div className="grid grid-cols-3 gap-3">
          <div>
            <p className="text-xs text-muted-foreground">Income</p>
            <p className="money mt-0.5 text-lg font-semibold text-positive">
              {formatCurrency(income)}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Expenses</p>
            <p className="money mt-0.5 text-lg font-semibold text-negative">
              {formatCurrency(expenses)}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Net</p>
            <p
              className={`money mt-0.5 text-lg font-semibold ${
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
              className="h-full bg-positive"
              style={{ width: `${incomePct}%` }}
            />
            <div className="h-full flex-1 bg-negative" />
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
          <div className="flex items-end justify-between gap-1.5 pt-1">
            {trend.map((m, i) => (
              <div
                key={i}
                className="flex flex-1 flex-col items-center gap-1.5"
                title={`${m.label}: ${m.net >= 0 ? "+" : "−"}${formatCurrency(Math.abs(m.net))}`}
              >
                <div className="flex h-10 w-full items-end justify-center">
                  <div
                    className={`w-full max-w-6 rounded-sm ${
                      m.net >= 0 ? "bg-positive/70" : "bg-negative/70"
                    }`}
                    style={{ height: `${Math.max(m.height * 100, 4)}%` }}
                  />
                </div>
                <span className="text-[10px] text-muted-foreground">
                  {m.label}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
