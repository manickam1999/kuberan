"use client";

import { useAuth } from "@/hooks/use-auth";
import { useAccountCurrencies } from "@/hooks/use-accounts";
import { formatMoney } from "@/lib/format";
import { Money } from "@/components/ui/money";
import type { Transaction } from "@/types/models";

interface TransactionAmountProps {
  transaction: Transaction;
  /**
   * The account page the row is shown on. A transfer into that account shows
   * only what it received, in its currency.
   */
  viewAccountId?: string;
  className?: string;
}

/**
 * A transaction's amount in its account's currency. A cross-currency transfer
 * reads `RM 4,460.00 → US$1,100.00` with the implied rate beneath.
 */
export function TransactionAmount({
  transaction,
  viewAccountId,
  className,
}: TransactionAmountProps) {
  const { hideBalances } = useAuth();
  const currencyOf = useAccountCurrencies();
  const currency = currencyOf(transaction.account_id, transaction.account);

  if (transaction.type === "transfer" && transaction.to_amount != null) {
    const toCurrency = currencyOf(
      transaction.to_account_id,
      transaction.to_account
    );
    if (viewAccountId && viewAccountId === transaction.to_account_id) {
      return (
        <Money
          amount={transaction.to_amount}
          currency={toCurrency}
          className={className}
        />
      );
    }
    return (
      <span className={className}>
        <span className="tabular-nums">
          {formatMoney(transaction.amount, currency, hideBalances)} →{" "}
          {formatMoney(transaction.to_amount, toCurrency, hideBalances)}
        </span>
        {transaction.implied_rate && !hideBalances && (
          <span className="block text-xs font-normal text-muted-foreground">
            @ {formatRate(transaction.implied_rate)}
          </span>
        )}
      </span>
    );
  }

  return (
    <Money
      amount={transaction.amount}
      currency={currency}
      className={className}
    />
  );
}

/** Trim an API decimal-string rate (10 dp) to a readable length. */
export function formatRate(rate: string): string {
  const n = parseFloat(rate);
  if (!Number.isFinite(n)) return rate;
  return n.toFixed(n >= 1 ? 4 : 6).replace(/0+$/, "").replace(/\.$/, "");
}
