import { cn } from "@/lib/utils";
import type { Budget, BudgetProgress } from "@/types/models";

/**
 * Shared visual language for budgets. The page cards, dialogs, and dashboard
 * card all consume these helpers so progress-bar coloring, the category chip,
 * and overspend copy stay identical everywhere and cannot drift.
 */

/** Semantic-token bar fill by usage (D8): primary <75%, warning 75-90%, negative >=90%. */
export function progressBarColor(percentage: number): string {
  if (percentage >= 90) return "bg-negative";
  if (percentage >= 75) return "bg-warning";
  return "bg-primary";
}

/** Match a budget id against a batch-progress list. */
export function findProgress(
  progress: BudgetProgress[] | undefined,
  budgetId: string
): BudgetProgress | undefined {
  return progress?.find((p) => p.budget_id === budgetId);
}

interface BudgetProgressBarProps {
  /** spent/budgeted*100, may exceed 100. */
  percentage: number;
  /** Bar height. Defaults to the page-card size. */
  size?: "sm" | "md";
  className?: string;
}

/**
 * The one progress bar used across the budgets page, dialogs, and dashboard
 * card. Width is capped at 100% (D8); color follows {@link progressBarColor}.
 */
export function BudgetProgressBar({
  percentage,
  size = "md",
  className,
}: BudgetProgressBarProps) {
  const width = Math.min(Math.max(percentage, 0), 100);
  return (
    <div
      className={cn(
        "w-full overflow-hidden rounded-full bg-muted",
        size === "sm" ? "h-1.5" : "h-2",
        className
      )}
    >
      <div
        className={cn(
          "h-full rounded-full transition-[width]",
          progressBarColor(percentage)
        )}
        style={{ width: `${width}%` }}
      />
    </div>
  );
}

interface CategoryChipProps {
  category?: Budget["category"];
  className?: string;
}

/**
 * Icon medallion tinted with the linked category's own color, matching the
 * category-chip treatment used on the categories page.
 */
export function BudgetCategoryChip({ category, className }: CategoryChipProps) {
  const color = category?.color;
  const label = category?.icon || category?.name?.charAt(0).toUpperCase() || "?";
  return (
    <span
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-lg text-sm",
        className
      )}
      style={{
        backgroundColor: color ? `${color}22` : "var(--muted)",
        color: color || "var(--muted-foreground)",
      }}
    >
      {label}
    </span>
  );
}
