"use client";

import { useMemo } from "react";
import { Money } from "@/components/ui/money";
import { useSpendingByCategory } from "@/hooks/use-transactions";
import { lastSixMonthsRange, sixMonthAverage } from "@/lib/budget-average";

/**
 * "You've averaged RM 912 a month on Shopping over the last six months."
 * Computed client-side from spending-by-category. The response is in the
 * user's base currency, so the hint is hidden for a budget in another
 * currency rather than comparing across currencies.
 */
export function BudgetAverageHint({
  categoryId,
  categoryName,
  currency,
}: {
  categoryId: string;
  categoryName: string;
  currency: string;
}) {
  const range = useMemo(() => lastSixMonthsRange(), []);
  const { data } = useSpendingByCategory(range.from, range.to);
  if (!data || !categoryId || data.currency !== currency) return null;
  const total =
    data.items.find((i) => i.category_id === categoryId)?.total ?? 0;
  if (total <= 0) {
    return (
      <p className="text-muted-foreground text-xs">
        No {categoryName} spending in the last six months.
      </p>
    );
  }
  return (
    <p className="text-muted-foreground text-xs">
      You&apos;ve averaged{" "}
      <Money
        amount={sixMonthAverage(total)}
        currency={currency}
        sign="never"
        className="text-foreground"
      />{" "}
      a month on {categoryName} over the last six months.
    </p>
  );
}
