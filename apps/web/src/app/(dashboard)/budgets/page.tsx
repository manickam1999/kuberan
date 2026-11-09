"use client";

import { useMemo, useState } from "react";
import { ChevronRight, Plus } from "lucide-react";
import { useBudgets, useBudgetsProgress } from "@/hooks/use-budgets";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { Money } from "@/components/ui/money";
import { CategoryIcon } from "@/components/categories/category-icon";
import { findProgress } from "@/components/budgets/budget-visuals";
import { BudgetCard } from "@/components/budgets/budget-card";
import {
  BudgetSummary,
  summarySentence,
  totalsByCurrency,
} from "@/components/budgets/budget-summary";
import { CreateBudgetDialog } from "@/components/budgets/create-budget-dialog";
import { EditBudgetDialog } from "@/components/budgets/edit-budget-dialog";
import { DeleteBudgetDialog } from "@/components/budgets/delete-budget-dialog";
import { periodDaysLeft } from "@/lib/budget-pace";
import type { Budget } from "@/types/models";

function eyebrow(now: Date): string {
  const month = now.toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  });
  const left = periodDaysLeft("monthly", now);
  return `${month} · ${left === 0 ? "last day" : `${left} ${left === 1 ? "day" : "days"} left`}`;
}

function PausedRow({ budget, onOpen }: { budget: Budget; onOpen: () => void }) {
  const name = budget.category?.name ?? budget.name;
  return (
    <button
      type="button"
      onClick={onOpen}
      className="hover:bg-accent/40 focus-visible:bg-accent/40 flex min-h-14 w-full items-center gap-3 px-4 py-2.5 text-left outline-none"
    >
      <Tile color={budget.category?.color || undefined} tone="neutral">
        <CategoryIcon icon={budget.category?.icon} name={name} />
      </Tile>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{name}</span>
        <span className="text-muted-foreground block truncate text-xs">
          {budget.period === "monthly" ? "Monthly" : "Yearly"} ·{" "}
          <Money amount={budget.amount} currency={budget.currency} sign="never" />
        </span>
      </span>
      <Badge tone="neutral">Paused</Badge>
      <ChevronRight className="text-muted-foreground size-4" aria-hidden />
    </button>
  );
}

function PageSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading budgets">
      <Skeleton className="h-36 w-full rounded-card" />
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-40 w-full rounded-card" />
        ))}
      </div>
    </div>
  );
}

export default function BudgetsPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const [editBudget, setEditBudget] = useState<Budget | null>(null);
  const [deleteBudget, setDeleteBudget] = useState<Budget | null>(null);

  const active = useBudgets({ is_active: true, page_size: 100 });
  const paused = useBudgets({ is_active: false, page_size: 100 });
  const progressQuery = useBudgetsProgress();
  const progressList = progressQuery.data;

  const now = useMemo(() => new Date(), []);
  const activeBudgets = useMemo(() => active.data?.data ?? [], [active.data]);
  const pausedBudgets = paused.data?.data ?? [];

  const activeProgress = useMemo(
    () =>
      (progressList ?? []).filter((p) =>
        activeBudgets.some((b) => b.id === p.budget_id)
      ),
    [progressList, activeBudgets]
  );
  const totals = useMemo(() => totalsByCurrency(activeProgress), [activeProgress]);
  const sentence = useMemo(
    () => summarySentence(activeBudgets, activeProgress),
    [activeBudgets, activeProgress]
  );

  // Most pressing first: highest share of the budget used.
  const sorted = useMemo(
    () =>
      [...activeBudgets].sort(
        (a, b) =>
          (findProgress(progressList, b.id)?.percentage ?? 0) -
          (findProgress(progressList, a.id)?.percentage ?? 0)
      ),
    [activeBudgets, progressList]
  );

  const isLoading = active.isLoading;
  const isError = active.isError || progressQuery.isError;
  const hasAny = activeBudgets.length > 0 || pausedBudgets.length > 0;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-muted-foreground text-xs">{eyebrow(now)}</p>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Budgets
          </h1>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          New budget
        </Button>
      </div>

      {isLoading ? (
        <PageSkeleton />
      ) : isError ? (
        <div
          role="alert"
          className="flex flex-col items-center rounded-card border border-dashed p-10 text-center"
        >
          <h3 className="text-base font-semibold">Couldn&apos;t load budgets</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Check your connection and try again.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            onClick={() => {
              void active.refetch();
              void progressQuery.refetch();
            }}
          >
            Try again
          </Button>
        </div>
      ) : !hasAny ? (
        <div className="flex flex-col items-center justify-center rounded-card border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No budgets yet</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Cap spending on a category and see at a glance whether you&apos;re on
            track.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create budget
          </Button>
        </div>
      ) : (
        <>
          {totals.length > 0 && (
            <BudgetSummary totals={totals} sentence={sentence} now={now} />
          )}

          {sorted.length > 0 && (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {sorted.map((budget) => (
                <BudgetCard
                  key={budget.id}
                  budget={budget}
                  progress={findProgress(progressList, budget.id)}
                  onOpen={() => setEditBudget(budget)}
                  now={now}
                />
              ))}
            </div>
          )}

          {pausedBudgets.length > 0 && (
            <section aria-labelledby="paused-heading" className="space-y-2">
              <div className="flex items-baseline gap-2">
                <h2 id="paused-heading" className="text-sm font-semibold">
                  Paused
                </h2>
                <span className="text-muted-foreground text-xs">
                  {pausedBudgets.length}
                </span>
              </div>
              <div className="bg-card divide-border/50 divide-y overflow-hidden rounded-card border">
                {pausedBudgets.map((b) => (
                  <PausedRow key={b.id} budget={b} onOpen={() => setEditBudget(b)} />
                ))}
              </div>
            </section>
          )}
        </>
      )}

      <CreateBudgetDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditBudgetDialog
        open={!!editBudget}
        onOpenChange={(open) => {
          if (!open) setEditBudget(null);
        }}
        budget={editBudget}
        onDelete={(b) => {
          setEditBudget(null);
          setDeleteBudget(b);
        }}
      />
      <DeleteBudgetDialog
        open={!!deleteBudget}
        onOpenChange={(open) => {
          if (!open) setDeleteBudget(null);
        }}
        budget={deleteBudget}
      />
    </div>
  );
}
