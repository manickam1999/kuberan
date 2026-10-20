import {
  Banknote,
  ClipboardList,
  PiggyBank,
  RotateCcw,
  Split,
  Users,
  type LucideIcon,
} from "lucide-react";
import { ApiClientError } from "@/lib/api-client";
import type {
  AccountType,
  Receivable,
  ReceivableKind,
  ReceivableStatus,
} from "@/types/models";
import { formatCurrency, formatMonthYear } from "@/lib/format";

// Re-exported so existing imports keep working; the helper lives in lib/format.
export { formatMonthYear };

/** Display labels for receivable kinds. */
export const RECEIVABLE_KIND_LABELS: Record<ReceivableKind, string> = {
  split: "Split",
  on_behalf: "Paid for someone",
  claim: "Claim",
  loan: "Loan",
  deposit: "Deposit",
  refund: "Refund",
};

/** Distinct icon per receivable kind (summary cards). */
export const RECEIVABLE_KIND_ICONS: Record<ReceivableKind, LucideIcon> = {
  split: Split,
  on_behalf: Users,
  claim: ClipboardList,
  loan: Banknote,
  deposit: PiggyBank,
  refund: RotateCcw,
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
const SETTLEMENT_ACCOUNT_TYPES: readonly AccountType[] = [
  "cash",
  "credit_card",
  "debt",
];

export function isSettlementAccountType(type: AccountType): boolean {
  return SETTLEMENT_ACCOUNT_TYPES.includes(type);
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

/**
 * Friendly copy for receivable-related API error codes, or null when the code
 * is not one of them (callers fall back to their own message).
 */
export function receivableErrorMessage(code: string): string | null {
  switch (code) {
    case "RECEIVABLE_HAS_PAYMENTS":
      return "Someone has already paid part of this. Undo their repayments from the Owed to me page before removing them.";
    case "INVALID_SHARES":
      return "The amounts owed back are not valid. Check the names, that nobody is listed twice, and that the total does not exceed the expense.";
    case "RECEIVABLE_HAS_SETTLEMENTS":
      return "This expense has repayments recorded. Undo them from the receivable's history on the Owed to me page first.";
    case "TRANSACTION_NOT_EDITABLE":
      return "This transaction can't be edited. Repayments are managed from the Owed to me page.";
    case "INVALID_TYPE_CHANGE":
      return "The type can't be changed while money is owed back on this expense.";
    case "SETTLEMENT_EXCEEDS_OUTSTANDING":
      return "That repayment is more than what is still owed. Enter an amount up to the outstanding balance.";
    case "WRITE_OFF_EXCEEDS_OUTSTANDING":
      return "You can't write off more than what is still owed.";
    case "INVALID_SETTLEMENT_ACCOUNT":
      return "Repayments can only be received into a cash, credit card or debt account.";
    case "RECEIVABLE_NOT_FOUND":
      return "This item no longer exists. It may have been removed; refresh and try again.";
    case "SETTLEMENT_NOT_FOUND":
      return "This repayment no longer exists. It may already have been undone.";
    case "INVALID_RECEIVABLE_KIND":
      return "That kind of receivable is not supported.";
    case "INVALID_INPUT":
      return "That input isn't valid. Names can't contain control characters, and an undo needs something to undo.";
    default:
      return null;
  }
}

/**
 * Single place to turn any thrown value into user-facing text: friendly copy
 * for known receivable codes, else the server message, else a generic line.
 */
export function receivableApiErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return receivableErrorMessage(error.code) ?? error.message;
  }
  return "Something went wrong";
}
