"use client";

import { useState } from "react";
import { ApiClientError } from "@/lib/api-client";
import { useWriteOff } from "@/hooks/use-receivables";
import { formatCurrency } from "@/lib/format";
import { writeOffImpactText } from "@/lib/receivable-format";
import { Button } from "@/components/ui/button";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import type { Receivable } from "@/types/models";

export interface WriteOffReceivableDialogProps {
  /** The receivable to forgive. Include `transaction` for the impact text. */
  receivable: Receivable;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Forgive part or all of the outstanding balance. The forgiven amount becomes
 * the user's spending, dated on the original expense. Amount defaults to the
 * whole outstanding balance (cents).
 */
export function WriteOffReceivableDialog({
  receivable,
  open,
  onOpenChange,
}: WriteOffReceivableDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <WriteOffForm
          receivable={receivable}
          onDone={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

function WriteOffForm({
  receivable,
  onDone,
}: {
  receivable: Receivable;
  onDone: () => void;
}) {
  const [amount, setAmount] = useState(receivable.outstanding);
  const [error, setError] = useState("");
  const writeOff = useWriteOff();

  const valid = amount > 0 && amount <= receivable.outstanding;
  const impact = valid ? writeOffImpactText(receivable, amount) : null;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!valid) {
      setError(
        `Amount must be between 0.01 and ${formatCurrency(receivable.outstanding)}`
      );
      return;
    }
    writeOff.mutate(
      {
        id: receivable.id,
        // Omit the amount for the full balance so the server uses its default.
        amount: amount === receivable.outstanding ? undefined : amount,
      },
      {
        onSuccess: onDone,
        onError: (err) =>
          setError(
            err instanceof ApiClientError ? err.message : "Something went wrong"
          ),
      }
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>Write off</DialogTitle>
        <DialogDescription>
          Forgive what {receivable.counterparty_name} still owes (
          {formatCurrency(receivable.outstanding)}). It becomes part of your own
          spending. You can undo this later.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="writeoff-amount">Amount to write off</Label>
        <CurrencyInput
          id="writeoff-amount"
          value={amount}
          onChange={setAmount}
          disabled={writeOff.isPending}
        />
        {impact && <p className="text-xs text-muted-foreground">{impact}</p>}
      </div>

      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onDone}
          disabled={writeOff.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={writeOff.isPending || !valid}>
          {writeOff.isPending ? "Saving..." : "Write off"}
        </Button>
      </DialogFooter>
    </form>
  );
}
