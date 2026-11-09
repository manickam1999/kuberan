"use client";

import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { CategoryIcon } from "@/components/categories/category-icon";
import { useSpendingByCategory } from "@/hooks/use-transactions";
import { useBaseCurrency } from "@/hooks/use-profile";
import { transactionsHref } from "@/lib/insights";

const MAX_ROWS = 8;

/** Spending by category, ranked; each row opens that category's expenses. */
export function CategoryBreakdownCard({
  fromDate,
  toDate,
}: {
  fromDate: string;
  toDate: string;
}) {
  const { data, isLoading, isError } = useSpendingByCategory(fromDate, toDate);
  const baseCurrency = useBaseCurrency();
  const currency = data?.currency ?? baseCurrency;

  const items = [...(data?.items ?? [])]
    .filter((i) => i.total > 0)
    .sort((a, b) => b.total - a.total);
  const shown = items.slice(0, MAX_ROWS);
  const rest = items.slice(MAX_ROWS).reduce((s, i) => s + i.total, 0);
  const max = Math.max(1, ...items.map((i) => i.total));

  return (
    <Card aria-busy={isLoading}>
      <CardHeader>
        <CardTitle className="text-base">By category</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-9 w-full" />
            ))}
          </div>
        ) : isError ? (
          <p role="alert" className="text-muted-foreground py-8 text-center text-sm">
            Couldn&apos;t load spending by category.
          </p>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground py-8 text-center text-sm">
            No spending recorded for this period.
          </p>
        ) : (
          <ul className="space-y-3">
            {shown.map((i) => (
              <li key={i.category_id ?? i.category_name}>
                <Link
                  href={transactionsHref({
                    from: fromDate,
                    to: toDate,
                    type: "expense",
                    categoryId: i.category_id,
                  })}
                  className="focus-visible:ring-ring/50 hover:bg-accent/40 -mx-2 grid grid-cols-[auto_1fr_auto] items-center gap-x-3 gap-y-1.5 rounded-md px-2 py-1 outline-none focus-visible:ring-[3px]"
                >
                  <Tile size="sm" color={i.category_color || undefined} tone="neutral">
                    <CategoryIcon icon={i.category_icon} name={i.category_name} />
                  </Tile>
                  <span className="truncate text-sm">{i.category_name}</span>
                  <Money
                    amount={i.total}
                    currency={currency}
                    sign="never"
                    className="money text-sm"
                  />
                  <span className="bg-muted col-span-3 block h-1.5 overflow-hidden rounded-full">
                    <span
                      className="block h-full rounded-full"
                      style={{
                        width: `${(i.total / max) * 100}%`,
                        backgroundColor: i.category_color || "var(--primary)",
                      }}
                    />
                  </span>
                </Link>
              </li>
            ))}
            {rest > 0 && (
              <li className="text-muted-foreground flex items-center justify-between text-sm">
                <span>{items.length - MAX_ROWS} more</span>
                <Money amount={rest} currency={currency} sign="never" />
              </li>
            )}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
