"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { CategoryIcon } from "@/components/categories/category-icon";
import { useTopExpenses, useTransaction } from "@/hooks/use-transactions";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { formatDate } from "@/lib/format";
import { transactionsHref } from "@/lib/insights";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";

const LIMIT = 6;

/**
 * The period's largest single expenses, ranked. Amounts are the effective
 * (personal-share) amount; a row in another currency shows its own amount with
 * the converted one beside it. A row opens the transaction.
 */
export function TopExpensesList({
  fromDate,
  toDate,
}: {
  fromDate: string;
  toDate: string;
}) {
  const baseCurrency = useBaseCurrency();
  const { data, isLoading, isError } = useTopExpenses(fromDate, toDate, LIMIT);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const { data: selectedTransaction } = useTransaction(selectedId ?? "");

  useEffect(() => {
    if (selectedTransaction && selectedTransaction.id === selectedId) {
      setEditOpen(true);
    }
  }, [selectedTransaction, selectedId]);

  const items = data?.items ?? [];
  const currency = data?.currency ?? baseCurrency;

  return (
    <Card aria-busy={isLoading}>
      <CardHeader>
        <CardTitle className="text-base">Largest expenses</CardTitle>
        <FxNotice fxMissing={data?.fx_missing} variant="inline" />
      </CardHeader>
      <CardContent className="flex flex-1 flex-col">
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: LIMIT }).map((_, i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : isError ? (
          <p role="alert" className="text-muted-foreground py-8 text-center text-sm">
            Couldn&apos;t load the largest expenses.
          </p>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground py-8 text-center text-sm">
            No expenses recorded for this period.
          </p>
        ) : (
          <ol className="-mx-2 mb-4">
            {items.map((item, index) => {
              const effective = item.personal_amount ?? item.amount;
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(item.id)}
                    className="hover:bg-accent/40 focus-visible:ring-ring/50 flex min-h-12 w-full items-center gap-3 rounded-md px-2 py-1.5 text-left outline-none focus-visible:ring-[3px]"
                  >
                    <span className="text-muted-foreground w-4 shrink-0 text-center text-xs tabular-nums">
                      {index + 1}
                    </span>
                    <Tile
                      size="sm"
                      color={item.category_color || undefined}
                      tone="neutral"
                    >
                      <CategoryIcon
                        icon={item.category_icon}
                        name={item.category_name}
                      />
                    </Tile>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm">
                        {item.description || item.category_name}
                      </span>
                      <span className="text-muted-foreground block truncate text-xs">
                        {formatDate(item.date)} · {item.account_name}
                      </span>
                    </span>
                    <span className="money shrink-0 text-right text-sm">
                      <Money
                        amount={effective}
                        currency={item.currency}
                        baseAmount={item.amount_base}
                        baseCurrency={currency}
                        fxMissing={item.fx_missing}
                        layout="stacked"
                        sign="never"
                      />
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
        )}
        {!isLoading && !isError && items.length > 0 && (
          // Pinned to the bottom, like the category card beside the calendar.
          <div className="text-muted-foreground mt-auto flex items-center justify-between gap-4 border-t pt-3 text-sm">
            <span>
              {data && data.total_count > items.length
                ? `Top ${items.length} of ${data.total_count} expenses`
                : `${items.length} ${items.length === 1 ? "expense" : "expenses"}`}
            </span>
            <Link
              href={transactionsHref({
                from: fromDate,
                to: toDate,
                type: "expense",
              })}
              className="text-primary focus-visible:ring-ring/50 rounded-sm outline-none hover:underline focus-visible:ring-[3px]"
            >
              See all →
            </Link>
          </div>
        )}
      </CardContent>

      <EditTransactionDialog
        open={editOpen}
        onOpenChange={(open) => {
          setEditOpen(open);
          if (!open) setSelectedId(null);
        }}
        transaction={selectedTransaction ?? null}
      />
    </Card>
  );
}
