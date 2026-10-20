import { formatCurrency } from "@/lib/format";
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
      className="h-4 shrink-0 px-1.5 text-[10px] font-medium"
    />
  );
}

/** Muted "Your share RM20" line shown under the amount of a shared expense. */
export function YourShareText({
  transaction,
  className,
}: {
  transaction: Transaction;
  className?: string;
}) {
  if (!isSharedExpense(transaction)) return null;
  const receivablesTotal = transaction.receivables!.reduce(
    (s, r) => s + r.amount,
    0
  );
  const share = transaction.personal_amount ?? transaction.amount - receivablesTotal;
  return (
    <span className={className ?? "block text-[11px] text-muted-foreground"}>
      Your share {formatCurrency(share)}
    </span>
  );
}
