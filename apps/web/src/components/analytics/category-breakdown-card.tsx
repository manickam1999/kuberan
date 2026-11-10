"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { CategoryIcon } from "@/components/categories/category-icon";
import { useSpendingByCategory } from "@/hooks/use-transactions";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useTypical } from "@/hooks/use-typical";
import { transactionsHref } from "@/lib/insights";
import {
  categoryComparison,
  categoryKey,
  hasTypical,
  typicalScale,
  vsTypicalLabel,
  vsTypicalTone,
} from "@/lib/typical";
import { cn } from "@/lib/utils";

const MAX_ROWS = 8;
/**
 * Below this many categories the list leaves room in the card, so each row
 * also shows how it compares with a typical month (no toggle needed).
 */
const INLINE_TYPICAL_BELOW = 5;

/** Spending by category, ranked; each row opens that category's expenses. */
export function CategoryBreakdownCard({
  fromDate,
  toDate,
  wholeMonth = false,
}: {
  fromDate: string;
  toDate: string;
  /** The period is one calendar month, so a typical month can be compared. */
  wholeMonth?: boolean;
}) {
  const { data, isLoading, isError } = useSpendingByCategory(fromDate, toDate);
  const typicalQuery = useTypical(6);
  const [view, setView] = useState<"amount" | "typical">("amount");
  const baseCurrency = useBaseCurrency();
  const currency = data?.currency ?? baseCurrency;

  const items = [...(data?.items ?? [])]
    .filter((i) => i.total > 0)
    .sort((a, b) => b.total - a.total);
  const shown = items.slice(0, MAX_ROWS);
  const rest = items.slice(MAX_ROWS).reduce((s, i) => s + i.total, 0);
  const max = Math.max(1, ...items.map((i) => i.total));

  // "vs typical" compares each category with its monthly average, scaled to
  // how far into the month it is when the period is the current month.
  const scale = wholeMonth
    ? typicalScale(typicalQuery.data, fromDate, new Date())
    : null;
  const typicalData = hasTypical(typicalQuery.data) ? typicalQuery.data : null;
  const comparison =
    typicalData && scale !== null
      ? categoryComparison(items, typicalData, scale)
      : null;
  const vsTypical = view === "typical" && comparison !== null;
  // A short list has room to show the comparison under each bar as well.
  const inlineTypical =
    !vsTypical && comparison !== null && items.length < INLINE_TYPICAL_BELOW;
  const showTicks = vsTypical || inlineTypical;
  // The bars share one scale; with the typical ticks it has to fit them too.
  const barMax = showTicks
    ? Math.max(max, ...[...comparison.values()].map((c) => c.typical))
    : max;

  return (
    <Card aria-busy={isLoading}>
      <CardHeader>
        <CardTitle className="text-base">By category</CardTitle>
        {comparison && (
          <CardAction>
            <Segmented
              aria-label="Category view"
              value={view}
              onValueChange={setView}
              options={[
                { value: "amount", label: "Amount" },
                { value: "typical", label: "vs typical" },
              ]}
            />
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="flex flex-1 flex-col">
        {isLoading ? (
          <div className="space-y-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-9 w-full" />
            ))}
          </div>
        ) : isError ? (
          <p
            role="alert"
            className="text-muted-foreground py-8 text-center text-sm"
          >
            Couldn&apos;t load spending by category.
          </p>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground py-8 text-center text-sm">
            No spending recorded for this period.
          </p>
        ) : (
          <ul className="mb-4 space-y-3">
            {shown.map((i) => {
              const cmp = comparison?.get(categoryKey(i));
              return (
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
                    <Tile
                      size="sm"
                      color={i.category_color || undefined}
                      tone="neutral"
                    >
                      <CategoryIcon
                        icon={i.category_icon}
                        name={i.category_name}
                      />
                    </Tile>
                    <span className="truncate text-sm">{i.category_name}</span>
                    {vsTypical && cmp ? (
                      <span
                        className={cn(
                          "money text-sm",
                          vsTypicalTone(cmp.pct) === "pos" && "text-positive",
                          vsTypicalTone(cmp.pct) === "neg" && "text-negative",
                          cmp.pct === null && "text-muted-foreground",
                        )}
                        title={
                          cmp.pct === null
                            ? "No history for this category"
                            : vsTypicalLabel(cmp.pct)
                        }
                      >
                        {cmp.pct === null
                          ? "New"
                          : cmp.pct === 0
                            ? "On par"
                            : `${cmp.pct < 0 ? "−" : "+"}${Math.abs(cmp.pct)}%`}
                      </span>
                    ) : (
                      <Money
                        amount={i.total}
                        currency={currency}
                        sign="never"
                        className="money text-sm"
                      />
                    )}
                    <span className="bg-muted relative col-span-3 block h-1.5 overflow-hidden rounded-full">
                      <span
                        className="block h-full rounded-full"
                        style={{
                          width: `${(i.total / barMax) * 100}%`,
                          backgroundColor: i.category_color || "var(--primary)",
                        }}
                      />
                      {showTicks && cmp && cmp.typical > 0 && (
                        <span
                          aria-hidden
                          className="bg-foreground/70 absolute top-0 h-full w-0.5"
                          style={{
                            left: `calc(${Math.min(100, (cmp.typical / barMax) * 100)}% - 1px)`,
                          }}
                        />
                      )}
                    </span>
                    {inlineTypical && cmp && (
                      <span
                        className={cn(
                          "col-span-3 text-xs",
                          vsTypicalTone(cmp.pct) === "pos"
                            ? "text-positive"
                            : vsTypicalTone(cmp.pct) === "neg"
                              ? "text-negative"
                              : "text-muted-foreground",
                        )}
                      >
                        {cmp.pct === null
                          ? "No history for this category"
                          : vsTypicalLabel(cmp.pct)}
                      </span>
                    )}
                  </Link>
                </li>
              );
            })}
            {rest > 0 && (
              <li className="text-muted-foreground flex items-center justify-between text-sm">
                <span>{items.length - MAX_ROWS} more</span>
                <Money amount={rest} currency={currency} sign="never" />
              </li>
            )}
          </ul>
        )}
        {!isLoading && !isError && items.length > 0 && (
          // Pinned to the bottom: when the card is taller than its list, the
          // spare room sits between the list and this line.
          <div className="text-muted-foreground mt-auto flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t pt-3 text-sm">
            <span>
              Total{" "}
              <Money
                amount={data?.total_spent ?? 0}
                currency={currency}
                sign="never"
              />{" "}
              · {items.length}{" "}
              {items.length === 1 ? "category" : "categories"}
            </span>
            <Link
              href={transactionsHref({
                from: fromDate,
                to: toDate,
                type: "expense",
              })}
              className="text-primary focus-visible:ring-ring/50 rounded-sm outline-none hover:underline focus-visible:ring-[3px]"
            >
              All expenses →
            </Link>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
