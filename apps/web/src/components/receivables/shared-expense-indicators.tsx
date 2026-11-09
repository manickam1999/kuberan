import { formatMoney } from "@/lib/format";
import { useAuth } from "@/hooks/use-auth";
import { useAccountCurrencies } from "@/hooks/use-accounts";
import { ReceivableKindBadge } from "./receivable-kind-badge";
import type { Transaction } from "@/types/models";

/** True for an expense that has money owed back (a shared expense). */
export function isSharedExpense(transaction: Transaction): boolean {
  return (
    transaction.type === "expense" && (transaction.receivables?.length ?? 0) > 0
  );
}

/** Compact kind badge for a shared expense row; renders nothing otherwise. */
export function SharedKindBadge({ transaction }: { transaction: Transaction }) {
  if (!isSharedExpense(transaction)) return null;
  const kind = transaction.receivables![0].kind;
  return (
    <ReceivableKindBadge
      kind={kind}
      className="shrink-0"
    />
  );
}

/**
 * Muted share line under the amount of a shared expense: "Your share RM 20",
 * shortened to "Share RM 20" on phones (the full label stays in the
 * accessibility tree), or "All owed back" when nothing is the user's own.
 */
export function YourShareText({
  transaction,
  className,
}: {
  transaction: Transaction;
  className?: string;
}) {
  const { hideBalances } = useAuth();
  const currencyOf = useAccountCurrencies();
  if (!isSharedExpense(transaction)) return null;
  const currency = currencyOf(transaction.account_id, transaction.account);
  const receivablesTotal = transaction.receivables!.reduce(
    (s, r) => s + r.amount,
    0
  );
  const share = transaction.personal_amount ?? transaction.amount - receivablesTotal;
  return (
    <span
      className={
        className ?? "block whitespace-nowrap text-[11px] text-muted-foreground"
      }
    >
      {share === 0 ? (
        "All owed back"
      ) : (
        <>
          <span aria-hidden="true" className="sm:hidden">
            Share
          </span>
          <span className="sr-only sm:not-sr-only">Your share</span>{" "}
          {formatMoney(share, currency, hideBalances)}
        </>
      )}
    </span>
  );
}
