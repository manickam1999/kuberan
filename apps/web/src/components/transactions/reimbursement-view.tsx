"use client";

import Link from "next/link";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { DialogFooter } from "@/components/ui/dialog";
import { formatCurrency, formatDate } from "@/lib/format";
import type { Transaction } from "@/types/models";

/**
 * Read-only view of a repayment (reimbursement transaction). Repayments are
 * created and undone from the Owed to me page, never edited here.
 */
export function ReimbursementView({
  transaction,
  accountName,
  currency,
  onNavigate,
}: {
  transaction: Transaction;
  accountName: string;
  currency?: string;
  /** Called when the user follows the link to Owed to me (closes the dialog). */
  onNavigate: () => void;
}) {
  return (
    <div className="flex flex-col gap-3">
      <p className="rounded-md bg-muted/50 p-3 text-sm text-muted-foreground">
        This is a repayment. To change or remove it, undo it from this
        person&apos;s History on the Owed to me page.
      </p>
      <div className="flex items-center justify-between">
        <Label>Amount</Label>
        <span className="money text-sm font-medium">
          {formatCurrency(transaction.amount, currency)}
        </span>
      </div>
      <div className="flex items-center justify-between">
        <Label>Received into</Label>
        <span className="text-sm">{accountName}</span>
      </div>
      <div className="flex items-center justify-between gap-4">
        <Label>Description</Label>
        <span className="text-right text-sm">
          {transaction.description || "—"}
        </span>
      </div>
      <div className="flex items-center justify-between">
        <Label>Date</Label>
        <span className="text-sm">{formatDate(transaction.date)}</span>
      </div>
      <DialogFooter>
        <Button asChild>
          <Link href="/receivables" onClick={onNavigate}>
            Go to Owed to me
          </Link>
        </Button>
      </DialogFooter>
    </div>
  );
}
