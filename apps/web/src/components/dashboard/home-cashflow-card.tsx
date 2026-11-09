"use client";

import { useMemo } from "react";
import { HomeCard } from "@/components/dashboard/home-bits";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import { Skeleton } from "@/components/ui/skeleton";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useMonthlySummary } from "@/hooks/use-transactions";

/** Cash flow: the month's in, out and share saved, with six months of bars. */
export function HomeCashflowCard() {
  const { data, isLoading, error } = useMonthlySummary(6);
  const active = useActiveMonth();
  const baseCurrency = useBaseCurrency();

  const view = useMemo(() => {
    const rows = data ?? [];
    const key = (() => {
      const d = new Date(active.fromDate);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
    })();
    const current =
      rows.find((r) => r.month === key) ??
      [...rows].reverse().find((r) => r.income > 0 || r.expenses > 0) ??
      rows[rows.length - 1];
    const max = Math.max(1, ...rows.flatMap((r) => [r.income, r.expenses]));
    return {
      income: current?.income ?? 0,
      expenses: current?.expenses ?? 0,
      currency: current?.currency ?? baseCurrency,
      bars: rows.map((r) => ({
        month: r.month,
        label: new Date(`${r.month}-01T00:00:00`).toLocaleDateString("en-US", {
          month: "short",
        }),
        income: r.income,
        expenses: r.expenses,
        incomeH: (r.income / max) * 100,
        expensesH: (r.expenses / max) * 100,
        isCurrent: current?.month === r.month,
      })),
      anyFxMissing: rows.some((r) => r.fx_missing),
    };
  }, [data, active.fromDate, baseCurrency]);

  const note = <span className="text-xs text-muted-foreground">6 months</span>;

  if (isLoading) {
    return (
      <HomeCard title="Cash flow" note={note}>
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-[88px] w-full" />
      </HomeCard>
    );
  }

  if (error) {
    return (
      <HomeCard title="Cash flow" note={note}>
        <p className="py-6 text-center text-sm text-destructive">
          Could not load cash flow.
        </p>
      </HomeCard>
    );
  }

  if (view.income === 0 && view.expenses === 0 && view.bars.length === 0) {
    return (
      <HomeCard title="Cash flow" note={note}>
        <p className="py-6 text-center text-sm text-muted-foreground">
          Income and spending appear here once you add transactions.
        </p>
      </HomeCard>
    );
  }

  const saved =
    view.income > 0
      ? Math.round(((view.income - view.expenses) / view.income) * 100)
      : null;

  return (
    <HomeCard title="Cash flow" note={note}>
      <div className="grid grid-cols-3 gap-3">
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">In</p>
          <Money
            amount={view.income}
            currency={view.currency}
            className="block whitespace-nowrap text-sm font-medium text-positive-text"
          />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">Out</p>
          <Money
            amount={view.expenses}
            currency={view.currency}
            className="block whitespace-nowrap text-sm font-medium"
          />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">Saved</p>
          <p className="text-sm font-medium tabular-nums">
            {saved === null ? "—" : `${saved < 0 ? "−" : ""}${Math.abs(saved)}%`}
          </p>
        </div>
      </div>

      <ul
        className="flex h-[88px] items-end gap-2 pt-1"
        aria-label="Income and spending by month"
      >
        {view.bars.map((b) => (
          <li
            key={b.month}
            className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1.5"
            title={`${b.label}: in ${(b.income / 100).toFixed(0)}, out ${(b.expenses / 100).toFixed(0)}`}
          >
            <div className="flex h-full w-full items-end justify-center gap-0.5">
              <span
                className="w-1/2 max-w-3 rounded-t-[3px] bg-positive/70"
                style={{ height: `${Math.max(b.incomeH, b.income > 0 ? 3 : 0)}%` }}
              />
              <span
                className="w-1/2 max-w-3 rounded-t-[3px] bg-muted-foreground/45"
                style={{
                  height: `${Math.max(b.expensesH, b.expenses > 0 ? 3 : 0)}%`,
                }}
              />
            </div>
            <span
              className={
                b.isCurrent
                  ? "text-[10px] font-medium text-foreground"
                  : "text-[10px] text-muted-foreground"
              }
            >
              {b.label}
            </span>
          </li>
        ))}
      </ul>
      <div className="flex items-center gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <i aria-hidden className="size-2 rounded-[2px] bg-positive/70" />
          In
        </span>
        <span className="flex items-center gap-1.5">
          <i aria-hidden className="size-2 rounded-[2px] bg-muted-foreground/45" />
          Out
        </span>
      </div>
      <FxNotice fxMissing={view.anyFxMissing} variant="inline" />
    </HomeCard>
  );
}
