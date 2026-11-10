"use client";

import { Money } from "@/components/ui/money";
import { Tile } from "@/components/ui/tile";
import { FxNotice } from "@/components/ui/fx-notice";
import { CategoryIcon } from "@/components/categories/category-icon";
import { BudgetPaceBar } from "./budget-pace-bar";
import { BudgetStatusChip } from "./budget-status-chip";
import {
  budgetStatus,
  periodDaysLeft,
  periodElapsedFraction,
} from "@/lib/budget-pace";
import type { Budget, BudgetProgress } from "@/types/models";

/**
 * One active budget: category tile, status chip, spent of budgeted, the bar
 * with its pace marker and a line on what is left. The whole card opens the
 * edit sheet (the name is the real button, stretched over the card).
 */
export function BudgetCard({
  budget,
  progress,
  onOpen,
  now,
}: {
  budget: Budget;
  progress?: BudgetProgress;
  onOpen: () => void;
  now: Date;
}) {
  const currency = budget.currency;
  const spent = progress?.spent ?? 0;
  const remaining = progress?.remaining ?? budget.amount;
  const percentage = progress?.percentage ?? 0;
  const status = budgetStatus(percentage);
  const elapsed = periodElapsedFraction(budget.period, now);
  const daysLeft = periodDaysLeft(budget.period, now);
  const unit = budget.period === "monthly" ? "month" : "year";
  const perDay =
    remaining > 0 && daysLeft > 0 ? Math.floor(remaining / daysLeft) : null;
  const name = budget.category?.name ?? budget.name;
  const dayWord = daysLeft === 1 ? "day" : "days";

  return (
    <div className="bg-card hover:border-input relative flex flex-col gap-3 rounded-card border p-4 transition-colors">
      <div className="flex items-center gap-3">
        <Tile color={budget.category?.color || undefined} tone="neutral">
          <CategoryIcon icon={budget.category?.icon} name={name} />
        </Tile>
        <div className="min-w-0 flex-1">
          <button
            type="button"
            onClick={onOpen}
            className="focus-visible:after:ring-ring/50 block max-w-full truncate rounded-sm text-left text-sm font-semibold outline-none after:absolute after:inset-0 after:rounded-card focus-visible:after:ring-[3px]"
            aria-label={`Edit ${name} budget`}
          >
            {name}
          </button>
          <p className="text-muted-foreground truncate text-xs">
            {budget.period === "monthly" ? "Monthly" : "Yearly"}
            {budget.name !== name ? ` · ${budget.name}` : ""}
          </p>
        </div>
        <BudgetStatusChip status={status} />
      </div>

      <div className="flex items-baseline justify-between gap-2">
        <Money
          amount={spent}
          currency={currency}
          size="md"
          sign="never"
          className="font-figures"
        />
        <span className="text-muted-foreground text-xs">
          of <Money amount={budget.amount} currency={currency} sign="never" />
        </span>
      </div>

      <BudgetPaceBar
        percentage={percentage}
        elapsed={elapsed}
        status={status}
        thick
        label={`${name}: ${Math.round(percentage)}% of budget used, ${Math.round(elapsed * 100)}% of the ${unit} gone`}
      />

      <p className="text-muted-foreground text-xs">
        {remaining < 0 ? (
          <>
            <span className="text-negative">
              <Money amount={-remaining} currency={currency} sign="never" /> over
            </span>
            {daysLeft > 0 ? ` · ${daysLeft} ${dayWord} left` : " · last day"}
          </>
        ) : (
          <>
            <Money amount={remaining} currency={currency} sign="never" /> left
            {perDay !== null && (
              <>
                {" · about "}
                <Money amount={perDay} currency={currency} sign="never" />
                {` a day for the last ${daysLeft} ${dayWord}`}
              </>
            )}
            {daysLeft === 0 && " · last day"}
          </>
        )}
      </p>
      <FxNotice fxMissing={progress?.fx_missing} variant="inline" />
    </div>
  );
}
