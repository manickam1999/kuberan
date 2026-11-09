"use client";

import { useMemo } from "react";
import Link from "next/link";
import { Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import { HomeCard, CategoryTile } from "@/components/dashboard/home-bits";
import { useBudgets, useBudgetsProgress } from "@/hooks/use-budgets";
import { budgetStatus } from "@/lib/home";
import {
  BudgetProgressBar,
  findProgress,
} from "@/components/budgets/budget-visuals";

const TOP_N = 4;

/** Budgets closest to (or past) their limit, most used first. */
export function BudgetsCard() {
  const {
    data: budgetsData,
    isLoading: budgetsLoading,
    error: budgetsError,
  } = useBudgets({ is_active: true, page_size: 100 });
  const {
    data: progressList,
    isLoading: progressLoading,
    error: progressError,
  } = useBudgetsProgress();

  const isLoading = budgetsLoading || progressLoading;

  const rows = useMemo(() => {
    const budgets = budgetsData?.data ?? [];
    return budgets
      .map((budget) => ({
        budget,
        progress: findProgress(progressList, budget.id),
      }))
      .sort(
        (a, b) => (b.progress?.percentage ?? 0) - (a.progress?.percentage ?? 0)
      )
      .slice(0, TOP_N);
  }, [budgetsData, progressList]);

  const anyFxMissing = rows.some((r) => r.progress?.fx_missing ?? false);

  if (isLoading) {
    return (
      <HomeCard title="Budgets" href="/budgets" linkLabel="All">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </HomeCard>
    );
  }

  if (budgetsError || progressError) {
    return (
      <HomeCard title="Budgets" href="/budgets" linkLabel="All">
        <p className="py-6 text-center text-sm text-destructive">
          Could not load budgets.
        </p>
      </HomeCard>
    );
  }

  if (rows.length === 0) {
    return (
      <HomeCard title="Budgets">
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-6 text-center">
          <p className="text-sm text-muted-foreground">
            No active budgets yet.
          </p>
          <Button asChild size="sm" className="mt-3">
            <Link href="/budgets">
              <Plus className="size-4" />
              Create budget
            </Link>
          </Button>
        </div>
      </HomeCard>
    );
  }

  return (
    <HomeCard title="Budgets" href="/budgets" linkLabel="All">
      <div className="space-y-3.5">
        {rows.map(({ budget, progress }) => {
          const percentage = progress?.percentage ?? 0;
          const spent = progress?.spent ?? 0;
          const status = budgetStatus(percentage);
          return (
            <Link
              key={budget.id}
              href="/budgets"
              className="-mx-2 block space-y-1.5 rounded-lg px-2 py-1 transition-colors hover:bg-accent/40 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <div className="flex items-center gap-2.5">
                <CategoryTile category={budget.category} />
                <span className="min-w-0 flex-1 truncate text-sm font-medium">
                  {budget.name}
                </span>
                {status === "over" ? (
                  <Badge tone="neg">
                    Over{" "}
                    <Money
                      amount={spent - budget.amount}
                      currency={budget.currency}
                      sign="never"
                    />
                  </Badge>
                ) : (
                  <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                    {status === "close" && <Badge tone="warn">Close</Badge>}
                    <span className="tabular-nums">
                      <Money amount={spent} currency={budget.currency} compact />
                      {" / "}
                      <Money
                        amount={budget.amount}
                        currency={budget.currency}
                        compact
                      />
                    </span>
                  </span>
                )}
              </div>
              <BudgetProgressBar percentage={percentage} size="sm" />
            </Link>
          );
        })}
      </div>
      <FxNotice fxMissing={anyFxMissing} variant="inline" />
    </HomeCard>
  );
}
