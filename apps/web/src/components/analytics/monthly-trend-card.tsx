"use client";

import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useMonthlySummary } from "@/hooks/use-transactions";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";
import { formatMoney } from "@/lib/format";
import { useAuth } from "@/hooks/use-auth";
import { monthBounds, savingsRate, transactionsHref } from "@/lib/insights";

const CHART_HEIGHT = 150;

/**
 * Income against spending for the last six months, as paired bars. Each month
 * links to that month's transactions.
 */
export function MonthlyTrendCard() {
  const { data, isLoading, isError } = useMonthlySummary(6);
  const baseCurrency = useBaseCurrency();
  const { hideBalances } = useAuth();
  const months = data ?? [];
  const currency = months[0]?.currency ?? baseCurrency;
  const max = Math.max(1, ...months.flatMap((m) => [m.income, m.expenses]));
  const totalIncome = months.reduce((s, m) => s + m.income, 0);
  const totalExpenses = months.reduce((s, m) => s + m.expenses, 0);
  const avgSaved = savingsRate(totalIncome, totalExpenses);
  const money = (cents: number) => formatMoney(cents, currency, hideBalances);

  return (
    <Card aria-busy={isLoading}>
      <CardHeader>
        <CardTitle className="text-base">Last 6 months</CardTitle>
        <FxNotice
          fxMissing={months.some((m) => m.fx_missing)}
          variant="inline"
        />
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading ? (
          <Skeleton className="h-[190px] w-full" />
        ) : isError ? (
          <p role="alert" className="text-muted-foreground py-8 text-center text-sm">
            Couldn&apos;t load the monthly trend.
          </p>
        ) : months.length === 0 ? (
          <p className="text-muted-foreground py-8 text-center text-sm">
            No activity in the last six months.
          </p>
        ) : (
          <div
            className="grid items-end gap-2"
            style={{
              gridTemplateColumns: `repeat(${months.length}, minmax(0, 1fr))`,
            }}
          >
            {months.map((m) => {
              const { from, to } = monthBounds(m.month);
              const [y, mo] = m.month.split("-").map(Number);
              const label = new Date(y, mo - 1, 1).toLocaleDateString("en-US", {
                month: "short",
              });
              return (
                <Link
                  key={m.month}
                  href={transactionsHref({ from, to })}
                  aria-label={`${label} ${y}: income ${money(m.income)}, spending ${money(m.expenses)}`}
                  className="focus-visible:ring-ring/50 hover:bg-accent/40 grid gap-1.5 rounded-md p-1 outline-none focus-visible:ring-[3px]"
                >
                  <span
                    className="flex items-end justify-center gap-1"
                    style={{ height: CHART_HEIGHT }}
                  >
                    <span
                      className="bg-positive w-full max-w-5 rounded-t-sm"
                      style={{
                        height: `${Math.max(2, (m.income / max) * CHART_HEIGHT)}px`,
                      }}
                    />
                    <span
                      className="bg-muted-foreground/50 w-full max-w-5 rounded-t-sm"
                      style={{
                        height: `${Math.max(2, (m.expenses / max) * CHART_HEIGHT)}px`,
                      }}
                    />
                  </span>
                  <span className="text-muted-foreground text-center text-xs">
                    {label}
                  </span>
                </Link>
              );
            })}
          </div>
        )}
        {months.length > 0 && (
          <div className="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
            <span className="inline-flex items-center gap-1.5">
              <i className="bg-positive size-2 rounded-[2px]" aria-hidden />
              Income
            </span>
            <span className="inline-flex items-center gap-1.5">
              <i className="bg-muted-foreground/50 size-2 rounded-[2px]" aria-hidden />
              Spending
            </span>
            {avgSaved !== null && (
              <span className="ml-auto">Avg saved {avgSaved}%</span>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
