"use client";

import { useMemo } from "react";
import Link from "next/link";
import { ArrowRight, Plus } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useBudgets, useBudgetsProgress } from "@/hooks/use-budgets";
import { formatCurrency, formatPercentage } from "@/lib/format";
import {
  BudgetProgressBar,
  BudgetCategoryChip,
  findProgress,
} from "@/components/budgets/budget-visuals";

const TOP_N = 4;

export function BudgetsCard() {
  const { data: budgetsData, isLoading: budgetsLoading } = useBudgets({
    is_active: true,
    page_size: 100,
  });
  const { data: progressList, isLoading: progressLoading } =
    useBudgetsProgress();

  const isLoading = budgetsLoading || progressLoading;

  // Top budgets by % used, joining the list to batch progress.
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

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Budgets</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </CardContent>
      </Card>
    );
  }

  if (rows.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Budgets</CardTitle>
        </CardHeader>
        <CardContent>
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
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">Budgets</CardTitle>
        <Button
          asChild
          variant="link"
          size="sm"
          className="h-auto p-0 text-muted-foreground"
        >
          <Link href="/budgets">
            View all <ArrowRight className="ml-1 size-3.5" />
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {rows.map(({ budget, progress }) => {
          const percentage = progress?.percentage ?? 0;
          const spent = progress?.spent ?? 0;
          return (
            <div key={budget.id} className="flex items-center gap-3">
              <BudgetCategoryChip
                category={budget.category}
                className="size-8"
              />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="truncate font-medium">{budget.name}</span>
                  <span className="money shrink-0 text-xs text-muted-foreground">
                    {formatCurrency(spent)} / {formatCurrency(budget.amount)}
                  </span>
                </div>
                <BudgetProgressBar percentage={percentage} size="sm" />
              </div>
              <span className="w-12 shrink-0 text-right text-xs text-muted-foreground">
                {formatPercentage(percentage)}
              </span>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
