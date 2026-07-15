"use client";

import { useState } from "react";
import {
  Pencil,
  Plus,
  Trash2,
  Wallet,
  TrendingDown,
  PiggyBank,
  Pause,
  Play,
} from "lucide-react";
import { useBudgets, useBudgetsProgress, useUpdateBudget } from "@/hooks/use-budgets";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { formatCurrency, formatPercentage } from "@/lib/format";
import {
  BudgetProgressBar,
  BudgetCategoryChip,
  findProgress,
} from "@/components/budgets/budget-visuals";
import { CreateBudgetDialog } from "@/components/budgets/create-budget-dialog";
import { EditBudgetDialog } from "@/components/budgets/edit-budget-dialog";
import { DeleteBudgetDialog } from "@/components/budgets/delete-budget-dialog";
import type { Budget, BudgetProgress } from "@/types/models";

type TabValue = "all" | "active" | "inactive";

function BudgetCard({
  budget,
  progress,
  onEdit,
  onDelete,
  onToggleActive,
  toggling,
}: {
  budget: Budget;
  progress?: BudgetProgress;
  onEdit: () => void;
  onDelete: () => void;
  onToggleActive: () => void;
  toggling: boolean;
}) {
  const spent = progress?.spent ?? 0;
  const remaining = progress?.remaining ?? budget.amount;
  const percentage = progress?.percentage ?? 0;
  const overBudget = remaining < 0;

  return (
    <Card className="flex flex-col gap-4 p-4">
      <div className="flex items-start gap-3">
        <BudgetCategoryChip category={budget.category} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{budget.name}</p>
          <p className="truncate text-xs text-muted-foreground">
            {budget.category?.name ?? "Uncategorized"}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Badge variant={budget.period === "monthly" ? "secondary" : "outline"}>
            {budget.period === "monthly" ? "Monthly" : "Yearly"}
          </Badge>
          {!budget.is_active && <Badge variant="outline">Paused</Badge>}
        </div>
      </div>

      <div className="space-y-2">
        <div className="flex items-baseline justify-between">
          <span className="money text-lg font-semibold">
            {formatCurrency(budget.amount)}
          </span>
          <span className="text-xs text-muted-foreground">
            {formatPercentage(percentage)}
          </span>
        </div>
        <BudgetProgressBar percentage={percentage} />
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">
            {formatCurrency(spent)} of {formatCurrency(budget.amount)}
          </span>
          <span className={overBudget ? "text-negative" : "text-positive"}>
            {overBudget
              ? `Over by ${formatCurrency(-remaining)}`
              : `${formatCurrency(remaining)} left`}
          </span>
        </div>
      </div>

      <div className="flex items-center justify-end gap-0.5 border-t pt-2">
        <Button
          variant="ghost"
          size="icon"
          className="size-8 text-muted-foreground"
          onClick={onToggleActive}
          disabled={toggling}
          title={budget.is_active ? "Pause" : "Resume"}
        >
          {budget.is_active ? (
            <Pause className="size-4" />
          ) : (
            <Play className="size-4" />
          )}
        </Button>
        <Button variant="ghost" size="icon" className="size-8" onClick={onEdit}>
          <Pencil className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-8 text-muted-foreground hover:text-destructive"
          onClick={onDelete}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    </Card>
  );
}

function BudgetCardWithActions({
  budget,
  onEdit,
  onDelete,
  progress,
}: {
  budget: Budget;
  onEdit: () => void;
  onDelete: () => void;
  progress?: BudgetProgress;
}) {
  const updateBudget = useUpdateBudget(budget.id);
  return (
    <BudgetCard
      budget={budget}
      progress={progress}
      onEdit={onEdit}
      onDelete={onDelete}
      onToggleActive={() =>
        updateBudget.mutate({ is_active: !budget.is_active })
      }
      toggling={updateBudget.isPending}
    />
  );
}

export default function BudgetsPage() {
  const [tab, setTab] = useState<TabValue>("all");
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [editBudget, setEditBudget] = useState<Budget | null>(null);
  const [deleteBudget, setDeleteBudget] = useState<Budget | null>(null);

  const isActiveFilter =
    tab === "all" ? undefined : tab === "active" ? true : false;

  const { data, isLoading } = useBudgets({ page, is_active: isActiveFilter });
  const { data: progressList } = useBudgetsProgress();

  const budgets = data?.data ?? [];
  const totalPages = data?.total_pages ?? 1;
  const currentPage = data?.page ?? 1;

  // Batch progress covers active budgets only; totals reflect what's tracked.
  const totalBudgeted = (progressList ?? []).reduce((s, p) => s + p.budgeted, 0);
  const totalSpent = (progressList ?? []).reduce((s, p) => s + p.spent, 0);
  const totalRemaining = totalBudgeted - totalSpent;

  const handleTabChange = (value: string) => {
    setTab(value as TabValue);
    setPage(1);
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Budgets</h1>
          <p className="text-sm text-muted-foreground">
            Set recurring spending caps and track them against this period
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          New budget
        </Button>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard
          label="Total budgeted"
          value={formatCurrency(totalBudgeted)}
          icon={PiggyBank}
          tone="primary"
        />
        <StatCard
          label="Spent this period"
          value={formatCurrency(totalSpent)}
          icon={TrendingDown}
          tone="warning"
        />
        <StatCard
          label="Remaining"
          value={formatCurrency(totalRemaining)}
          icon={Wallet}
          tone={totalRemaining < 0 ? "negative" : "positive"}
        />
      </div>

      <Tabs value={tab} onValueChange={handleTabChange}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
          <TabsTrigger value="inactive">Inactive</TabsTrigger>
        </TabsList>
      </Tabs>

      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-44 w-full rounded-xl" />
          ))}
        </div>
      ) : budgets.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No budgets yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first budget to cap spending on an expense category.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create budget
          </Button>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
          {budgets.map((budget) => (
            <BudgetCardWithActions
              key={budget.id}
              budget={budget}
              progress={findProgress(progressList, budget.id)}
              onEdit={() => setEditBudget(budget)}
              onDelete={() => setDeleteBudget(budget)}
            />
          ))}
        </div>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Page {currentPage} of {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={currentPage <= 1}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage >= totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}

      <CreateBudgetDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditBudgetDialog
        open={!!editBudget}
        onOpenChange={(open) => {
          if (!open) setEditBudget(null);
        }}
        budget={editBudget}
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
