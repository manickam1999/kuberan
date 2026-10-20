"use client";

import { useState } from "react";
import { Undo2 } from "lucide-react";
import { useAccounts } from "@/hooks/use-accounts";
import { useDeleteSettlement, useReceivable } from "@/hooks/use-receivables";
import { formatCurrency, formatDate } from "@/lib/format";
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
import { Skeleton } from "@/components/ui/skeleton";
import type { ReceivableSettlement } from "@/types/models";

export interface ReceivableSettlementHistoryProps {
  /** Receivable whose settlements to show (fetched via GET /receivables/:id). */
  receivableId: string;
}

/** Repayment history for one receivable, with a confirmed "Undo" per row. */
export function ReceivableSettlementHistory({
  receivableId,
}: ReceivableSettlementHistoryProps) {
  const { data: receivable, isLoading } = useReceivable(receivableId);
  const { data: accountData } = useAccounts({ page_size: 100 });
  const deleteSettlement = useDeleteSettlement();
  const [pending, setPending] = useState<ReceivableSettlement | null>(null);

  const accountName = new Map((accountData?.data ?? []).map((a) => [a.id, a.name]));

  if (isLoading) return <Skeleton className="h-10 w-full" />;
  const settlements = receivable?.settlements ?? [];
  if (settlements.length === 0) {
    return <p className="text-xs text-muted-foreground">No repayments yet.</p>;
  }

  return (
    <>
      <ul className="divide-y divide-border/50 text-sm">
        {settlements.map((s) => (
          <li key={s.id} className="flex items-center justify-between gap-3 py-2">
            <div className="min-w-0">
              <div className="money font-medium">{formatCurrency(s.amount)}</div>
              <div className="truncate text-xs text-muted-foreground">
                {formatDate(s.date)} · {accountName.get(s.account_id) ?? "Account"}
              </div>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setPending(s)}
              disabled={deleteSettlement.isPending}
            >
              <Undo2 className="size-4" />
              Undo
            </Button>
          </li>
        ))}
      </ul>

      <AlertDialog
        open={pending !== null}
        onOpenChange={(o) => {
          if (!o) setPending(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Undo this repayment?</AlertDialogTitle>
            <AlertDialogDescription>
              {pending ? formatCurrency(pending.amount) : ""} will be removed from
              the account it was credited to, and the amount becomes outstanding
              again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pending) deleteSettlement.mutate(pending.id);
                setPending(null);
              }}
            >
              Undo repayment
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
