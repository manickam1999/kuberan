import type {
  AccountType,
  Receivable,
  ReceivableKind,
  ReceivableStatus,
} from "@/types/models";
import { formatCurrency } from "@/lib/format";

/** Display labels for receivable kinds. */
export const RECEIVABLE_KIND_LABELS: Record<ReceivableKind, string> = {
  split: "Split",
  on_behalf: "Paid for someone",
  claim: "Claim",
  loan: "Loan",
  deposit: "Deposit",
  refund: "Refund",
};

export const RECEIVABLE_KINDS = Object.keys(
  RECEIVABLE_KIND_LABELS
) as ReceivableKind[];

/** Display labels for receivable statuses. */
export const RECEIVABLE_STATUS_LABELS: Record<ReceivableStatus, string> = {
  pending: "Pending",
  partially_paid: "Partially paid",
  settled: "Settled",
  written_off: "Written off",
};

/** Account types a repayment may be credited to (D9). */
export const SETTLEMENT_ACCOUNT_TYPES: readonly AccountType[] = [
  "cash",
  "credit_card",
  "debt",
];

export function isSettlementAccountType(type: AccountType): boolean {
  return SETTLEMENT_ACCOUNT_TYPES.includes(type);
}

/** "March 2026" for an ISO date, without timezone-induced day shifts. */
export function formatMonthYear(iso: string): string {
  const [year, month] = iso.split("T")[0].split("-").map(Number);
  return new Date(year, month - 1, 1).toLocaleDateString("en-US", {
    month: "long",
    year: "numeric",
  });
}

/**
 * Impact text for forgiving part of a receivable, e.g.
 * "Adds $150.00 to March 2026 spending". Returns null when there is nothing
 * to write off. Falls back to "your spending" when the expense date is unknown.
 */
export function writeOffImpactText(
  receivable: Receivable,
  writeOffCents: number
): string | null {
  if (writeOffCents <= 0) return null;
  const when = receivable.transaction
    ? `${formatMonthYear(receivable.transaction.date)} spending`
    : "your spending";
  return `Adds ${formatCurrency(writeOffCents)} to ${when}`;
}

/** True when the receivable still has money that can be collected or forgiven. */
export function isOpen(receivable: Receivable): boolean {
  return receivable.outstanding > 0;
}
