"use client";

import { useMemo } from "react";
import { YourShareText } from "@/components/receivables/shared-expense-indicators";
import { TransactionAmount } from "@/components/transactions/transaction-amount";
import { HomeCard, CategoryTile } from "@/components/dashboard/home-bits";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { transactionVisual } from "@/lib/domain-visuals";
import { groupByDay } from "@/lib/home";
import { cn } from "@/lib/utils";
import type { Account, Transaction } from "@/types/models";

interface HomeRecentProps {
  transactions: readonly Transaction[];
  accounts: readonly Account[];
  baseCurrency: string;
  isLoading: boolean;
  error: unknown;
  onSelect: (transaction: Transaction) => void;
}

function RecentRow({
  transaction,
  accountName,
  currency,
  onSelect,
}: {
  transaction: Transaction;
  accountName?: string;
  currency: string;
  onSelect: (transaction: Transaction) => void;
}) {
  const v = transactionVisual(transaction.type);
  const subtitle = [transaction.category?.name ?? v.label, accountName]
    .filter(Boolean)
    .join(" · ");
  // Spending reads in the body colour; only money in is coloured, and the
  // sign carries the direction as well.
  const signed =
    v.sign === "-" ? -transaction.amount : transaction.amount;

  return (
    <button
      type="button"
      onClick={() => onSelect(transaction)}
      className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-3 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent/50 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <CategoryTile category={transaction.category} type={transaction.type} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">
          {transaction.description || v.label}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {subtitle}
        </span>
      </span>
      <span className="shrink-0 text-right text-sm font-medium">
        {transaction.type === "transfer" ? (
          <TransactionAmount
            transaction={transaction}
            className={v.text}
          />
        ) : (
          <Money
            amount={signed}
            currency={currency}
            sign={v.sign === "+" ? "always" : v.sign === "-" ? "auto" : "never"}
            className={cn("block", v.text)}
          />
        )}
        <YourShareText transaction={transaction} />
      </span>
    </button>
  );
}

/** The last few transactions, grouped by day; a row opens the transaction. */
export function HomeRecent({
  transactions,
  accounts,
  baseCurrency,
  isLoading,
  error,
  onSelect,
}: HomeRecentProps) {
  const groups = useMemo(() => groupByDay(transactions), [transactions]);
  const names = useMemo(
    () => new Map(accounts.map((a) => [a.id, a])),
    [accounts]
  );

  return (
    <HomeCard
      title="Recent"
      href="/transactions"
      linkLabel="All transactions"
    >
      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-11 w-full" />
          ))}
        </div>
      ) : error ? (
        <p className="py-6 text-center text-sm text-destructive">
          Could not load recent transactions.
        </p>
      ) : groups.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">
          No transactions yet. Add one with the New button.
        </p>
      ) : (
        <div className="space-y-2">
          {groups.map((g) => (
            <section key={g.key} aria-label={g.label}>
              <h3 className="py-1 font-mono text-[11px] uppercase tracking-[0.1em] text-muted-foreground">
                {g.label}
              </h3>
              <div>
                {g.items.map((tx) => (
                  <RecentRow
                    key={tx.id}
                    transaction={tx}
                    accountName={names.get(tx.account_id)?.name}
                    currency={names.get(tx.account_id)?.currency ?? baseCurrency}
                    onSelect={onSelect}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </HomeCard>
  );
}
