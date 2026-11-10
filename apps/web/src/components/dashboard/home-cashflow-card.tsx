"use client";

import { useMemo, useState } from "react";
import { HomeCard } from "@/components/dashboard/home-bits";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import { Skeleton } from "@/components/ui/skeleton";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useMonthlySummary } from "@/hooks/use-transactions";
import { monthFlow } from "@/lib/home";
import { cn } from "@/lib/utils";

/**
 * Cash flow: a month's in, out and what was saved (or overspent), with six
 * months of bars. Hovering, focusing or tapping a month shows its figures;
 * a tap sticks until tapped again, which is how it works on touch screens.
 */
export function HomeCashflowCard() {
  const { data, isLoading, error } = useMonthlySummary(6);
  const active = useActiveMonth();
  const baseCurrency = useBaseCurrency();
  const [hovered, setHovered] = useState<string | null>(null);
  const [pinned, setPinned] = useState<string | null>(null);

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
      currentMonth: current?.month ?? null,
      bars: rows.map((r) => ({
        month: r.month,
        label: new Date(`${r.month}-01T00:00:00`).toLocaleDateString("en-US", {
          month: "short",
        }),
        longLabel: new Date(`${r.month}-01T00:00:00`).toLocaleDateString(
          "en-US",
          { month: "long", year: "numeric" }
        ),
        income: r.income,
        expenses: r.expenses,
        currency: r.currency ?? baseCurrency,
        incomeH: (r.income / max) * 100,
        expensesH: (r.expenses / max) * 100,
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

  if (view.bars.length === 0) {
    return (
      <HomeCard title="Cash flow" note={note}>
        <p className="py-6 text-center text-sm text-muted-foreground">
          Income and spending appear here once you add transactions.
        </p>
      </HomeCard>
    );
  }

  // The month on show: hovered/focused, else tapped, else the current one.
  const shownMonth = hovered ?? pinned ?? view.currentMonth;
  const shown =
    view.bars.find((b) => b.month === shownMonth) ??
    view.bars[view.bars.length - 1];
  const flow = monthFlow(shown.income, shown.expenses);
  const isCurrent = shown.month === view.currentMonth;

  return (
    <HomeCard
      title="Cash flow"
      note={
        isCurrent ? (
          note
        ) : (
          <span className="text-xs font-medium text-foreground">
            {shown.longLabel}
          </span>
        )
      }
    >
      <div className="grid grid-cols-3 gap-3" aria-live="polite">
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">In</p>
          <Money
            amount={shown.income}
            currency={shown.currency}
            className="block whitespace-nowrap text-sm font-medium text-positive-text"
          />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">Out</p>
          <Money
            amount={shown.expenses}
            currency={shown.currency}
            className="block whitespace-nowrap text-sm font-medium"
          />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">
            {flow.overspent ? "Overspent" : "Saved"}
          </p>
          <Money
            amount={Math.abs(flow.net)}
            currency={shown.currency}
            className={cn(
              "block whitespace-nowrap text-sm font-medium",
              flow.overspent && "text-negative"
            )}
          />
          {flow.savedPercent !== null && (
            <p className="text-xs text-muted-foreground tabular-nums">
              {flow.savedPercent}% of income
            </p>
          )}
        </div>
      </div>

      <div
        className="flex h-[88px] items-end gap-2 pt-1"
        role="group"
        aria-label="Income and spending by month. Choose a month to see its figures."
        onMouseLeave={() => setHovered(null)}
      >
        {view.bars.map((b) => {
          const active = b.month === shown.month;
          return (
            <button
              key={b.month}
              type="button"
              aria-pressed={pinned === b.month}
              aria-label={b.longLabel}
              onMouseEnter={() => setHovered(b.month)}
              onFocus={() => setHovered(b.month)}
              onBlur={() => setHovered(null)}
              onClick={() =>
                setPinned((p) => (p === b.month ? null : b.month))
              }
              className={cn(
                "flex h-full min-w-0 flex-1 cursor-pointer flex-col items-center justify-end gap-1.5 rounded-md px-0.5 pt-1 outline-none transition-colors",
                "focus-visible:ring-2 focus-visible:ring-ring",
                active ? "bg-muted/60" : "hover:bg-muted/40"
              )}
            >
              <span className="flex h-full w-full items-end justify-center gap-0.5">
                <span
                  className="w-1/2 max-w-3 rounded-t-[3px] bg-positive/70"
                  style={{
                    height: `${Math.max(b.incomeH, b.income > 0 ? 3 : 0)}%`,
                  }}
                />
                <span
                  className="w-1/2 max-w-3 rounded-t-[3px] bg-muted-foreground/45"
                  style={{
                    height: `${Math.max(b.expensesH, b.expenses > 0 ? 3 : 0)}%`,
                  }}
                />
              </span>
              <span
                className={cn(
                  "text-[10px]",
                  active
                    ? "font-medium text-foreground"
                    : "text-muted-foreground"
                )}
              >
                {b.label}
              </span>
            </button>
          );
        })}
      </div>
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
