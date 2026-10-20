"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, HandCoins, Undo2, Eraser } from "lucide-react";
import { toast } from "sonner";
import { useUndoWriteOff } from "@/hooks/use-receivables";
import { formatCurrency } from "@/lib/format";
import { isOpen, receivableApiErrorMessage } from "@/lib/receivable-format";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { ReceivableKindBadge } from "./receivable-kind-badge";
import { ReceivableSettlementHistory } from "./receivable-settlement-history";
import { ReceivableStatusBadge } from "./receivable-status-badge";
import { SettleReceivableDialog } from "./settle-receivable-dialog";
import { WriteOffReceivableDialog } from "./write-off-receivable-dialog";
import type { Receivable, ReceivableExpense } from "@/types/models";

export interface ReceivableListProps {
  receivables: Receivable[];
  /**
   * The parent expense. Pass it when the receivables come embedded in a
   * Transaction (they lack `transaction`), so dialogs can default the account
   * and show the write-off month impact.
   */
  expense?: ReceivableExpense;
  /** Show each row's counterparty name. Pass false under a person group. Default true. */
  showCounterparty?: boolean;
  /** Show the expense description on each row. Pass false inside an expense. Default true. */
  showExpense?: boolean;
}

/**
 * Compact per-counterparty list: name, kind, amount, paid / outstanding and
 * status, with Record repayment / Write off / Undo write-off actions and an
 * expandable repayment history.
 */
export function ReceivableList({
  receivables,
  expense,
  showCounterparty = true,
  showExpense = true,
}: ReceivableListProps) {
  return (
    <div className="divide-y divide-border/50">
      {receivables.map((r) => (
        <ReceivableRow
          key={r.id}
          receivable={expense && !r.transaction ? { ...r, transaction: expense } : r}
          showCounterparty={showCounterparty}
          showExpense={showExpense}
        />
      ))}
    </div>
  );
}

function ReceivableRow({
  receivable,
  showCounterparty,
  showExpense,
}: {
  receivable: Receivable;
  showCounterparty: boolean;
  showExpense: boolean;
}) {
  const [settleOpen, setSettleOpen] = useState(false);
  const [writeOffOpen, setWriteOffOpen] = useState(false);
  const [undoOpen, setUndoOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const undoWriteOff = useUndoWriteOff();

  const open = isOpen(receivable);
  const hasPayments = receivable.paid_amount > 0;
  const description = showExpense ? receivable.transaction?.description : undefined;

  async function undo() {
    try {
      await undoWriteOff.mutateAsync(receivable.id);
      toast.success("Write-off undone");
    } catch (err) {
      toast.error(receivableApiErrorMessage(err));
    }
  }

  return (
    <div className="py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            {showCounterparty && (
              <span className="truncate text-sm font-medium">
                {receivable.counterparty_name}
              </span>
            )}
            <ReceivableKindBadge kind={receivable.kind} />
            <ReceivableStatusBadge status={receivable.status} />
          </div>
          <div className="mt-0.5 break-words text-xs text-muted-foreground">
            {description ? `${description} · ` : ""}
            Paid {formatCurrency(receivable.paid_amount)}
            {receivable.written_off_amount > 0 &&
              ` · Written off ${formatCurrency(receivable.written_off_amount)}`}
          </div>
        </div>
        <div className="shrink-0 text-right">
          <div className="money text-sm font-semibold">
            {formatCurrency(receivable.outstanding)}
          </div>
          <div className="text-xs text-muted-foreground">
            of {formatCurrency(receivable.amount)}
          </div>
        </div>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        {open && (
          <>
            <Button
              size="sm"
              variant="outline"
              className="h-10 sm:h-9"
              onClick={() => setSettleOpen(true)}
            >
              <HandCoins className="size-4" />
              Record repayment
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-10 sm:h-9"
              onClick={() => setWriteOffOpen(true)}
            >
              <Eraser className="size-4" />
              Write off
            </Button>
          </>
        )}
        {receivable.written_off_amount > 0 && (
          <Button
            size="sm"
            variant="ghost"
            className="h-10 sm:h-9"
            disabled={undoWriteOff.isPending}
            onClick={() => setUndoOpen(true)}
          >
            <Undo2 className="size-4" />
            Undo write-off
          </Button>
        )}
        {hasPayments && (
          <Button
            size="sm"
            variant="ghost"
            className="h-10 sm:h-9"
            onClick={() => setHistoryOpen((v) => !v)}
            aria-expanded={historyOpen}
          >
            {historyOpen ? (
              <ChevronDown className="size-4" />
            ) : (
              <ChevronRight className="size-4" />
            )}
            History
          </Button>
        )}
      </div>

      {historyOpen && hasPayments && (
        <div className="mt-2 rounded-md bg-muted/40 px-3 py-1">
          <ReceivableSettlementHistory receivableId={receivable.id} />
        </div>
      )}

      {open && (
        <>
          <SettleReceivableDialog
            receivable={receivable}
            open={settleOpen}
            onOpenChange={setSettleOpen}
          />
          <WriteOffReceivableDialog
            receivable={receivable}
            open={writeOffOpen}
            onOpenChange={setWriteOffOpen}
          />
        </>
      )}

      <AlertDialog open={undoOpen} onOpenChange={setUndoOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Undo this write-off?</AlertDialogTitle>
            <AlertDialogDescription>
              {formatCurrency(receivable.written_off_amount)} will be owed to you
              again by {receivable.counterparty_name}, and it will no longer count
              as your own spending.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void undo()}>
              Undo write-off
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
