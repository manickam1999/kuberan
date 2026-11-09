"use client";

import { Badge } from "@/components/ui/badge";
import { Money } from "@/components/ui/money";
import { BudgetPaceBar } from "./budget-pace-bar";
import { budgetStatus, periodElapsedFraction } from "@/lib/budget-pace";
import type { Budget, BudgetProgress } from "@/types/models";

export interface CurrencyTotals {
  currency: string;
  budgeted: number;
  spent: number;
}

/** Totals per budget currency: amounts in different currencies are never added. */
export function totalsByCurrency(
  progress: readonly BudgetProgress[]
): CurrencyTotals[] {
  const map = new Map<string, CurrencyTotals>();
  for (const p of progress) {
    const t = map.get(p.currency) ?? {
      currency: p.currency,
      budgeted: 0,
      spent: 0,
    };
    t.budgeted += p.budgeted;
    t.spent += p.spent;
    map.set(p.currency, t);
  }
  return [...map.values()];
}

function nameList(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

/** "Shopping is over. Groceries and Dining are under." */
export function summarySentence(
  budgets: readonly Budget[],
  progress: readonly BudgetProgress[]
): string {
  const over: string[] = [];
  const under: string[] = [];
  for (const b of budgets) {
    const p = progress.find((x) => x.budget_id === b.id);
    if (!p) continue;
    const name = b.category?.name ?? b.name;
    (p.percentage > 100 ? over : under).push(name);
  }
  const parts: string[] = [];
  if (over.length > 0)
    parts.push(`${nameList(over)} ${over.length === 1 ? "is" : "are"} over.`);
  if (under.length > 0)
    parts.push(`${nameList(under)} ${under.length === 1 ? "is" : "are"} under.`);
  return parts.join(" ");
}

/**
 * "Am I OK this month?": spent against everything budgeted, one block per
 * currency, with the pace marker for the monthly cycle.
 */
export function BudgetSummary({
  totals,
  sentence,
  now,
}: {
  totals: CurrencyTotals[];
  sentence: string;
  now: Date;
}) {
  const elapsed = periodElapsedFraction("monthly", now);
  return (
    <div className="bg-card space-y-4 rounded-card border p-4 md:p-5">
      {totals.map((t) => {
        const pct = t.budgeted > 0 ? (t.spent / t.budgeted) * 100 : 0;
        const status = budgetStatus(pct);
        const diff = t.budgeted - t.spent;
        return (
          <div key={t.currency} className="space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div>
                <span className="text-muted-foreground text-xs">
                  Spent against budgets
                </span>
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <Money
                    amount={t.spent}
                    currency={t.currency}
                    size="lg"
                    sign="never"
                    className="font-figures"
                  />
                  <span className="text-muted-foreground text-sm">
                    of{" "}
                    <Money
                      amount={t.budgeted}
                      currency={t.currency}
                      sign="never"
                    />
                  </span>
                </div>
              </div>
              <Badge
                tone={diff < 0 ? "neg" : status === "close" ? "warn" : "pos"}
              >
                <Money
                  amount={Math.abs(diff)}
                  currency={t.currency}
                  sign="never"
                />
                {diff < 0 ? " over" : " left"}
              </Badge>
            </div>
            <BudgetPaceBar
              percentage={pct}
              elapsed={elapsed}
              status={status}
              thick
              label={`Spent ${Math.round(pct)}% of everything budgeted in ${t.currency}`}
            />
          </div>
        );
      })}
      {sentence && <p className="text-muted-foreground text-sm">{sentence}</p>}
    </div>
  );
}
