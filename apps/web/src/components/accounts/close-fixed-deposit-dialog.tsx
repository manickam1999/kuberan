"use client";

import { useState, useEffect } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useCloseFixedDeposit, useAccounts } from "@/hooks/use-accounts";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatCurrency, formatDate, formatPayoutType } from "@/lib/format";
import type { Account } from "@/types/models";

interface CloseFixedDepositDialogProps {
  account: Account;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called after a successful close/payout. Use this to navigate away from pages that 404 after close. */
  onClosed?: () => void;
}

function estimateInterest(account: Account): number {
  const fd = account.fixed_deposit;
  if (!fd) return 0;
  if (fd.payout_type === "periodic") return 0;
  if (fd.status === "matured") return fd.maturity_value - fd.principal;
  // active / early close: pro-rata simple interest
  const rate = (account.interest_rate ?? 0) / 100;
  const startDate = new Date(fd.start_date.split("T")[0]);
  const today = new Date();
  const daysElapsed = Math.max(
    0,
    Math.floor((today.getTime() - startDate.getTime()) / (1000 * 60 * 60 * 24))
  );
  return Math.round(fd.principal * rate * (daysElapsed / 365));
}

export function CloseFixedDepositDialog({
  account,
  open,
  onOpenChange,
  onClosed,
}: CloseFixedDepositDialogProps) {
  const [destinationAccountId, setDestinationAccountId] = useState("");
  const [interestOverride, setInterestOverride] = useState<number | null>(null);
  const [penalty, setPenalty] = useState(0);
  const [error, setError] = useState("");

  const closeFD = useCloseFixedDeposit(account.id);
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const cashAccounts = (accountsData?.data ?? []).filter(
    (a) => a.type === "cash" && a.is_active
  );

  const fd = account.fixed_deposit;
  const isMatured = fd?.status === "matured";
  const isEarlyClose = fd?.status === "active";
  const estimatedInterest = estimateInterest(account);
  const displayInterest = interestOverride ?? estimatedInterest;
  const principal = fd?.principal ?? 0;
  const amountReceived = principal + displayInterest - penalty;

  useEffect(() => {
    if (open && fd?.linked_account_id) {
      setDestinationAccountId(fd.linked_account_id);
    }
  }, [open, fd?.linked_account_id]);

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setDestinationAccountId("");
      setInterestOverride(null);
      setPenalty(0);
      setError("");
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!fd) return;
    setError("");

    closeFD.mutate(
      {
        destination_account_id: destinationAccountId || undefined,
        interest_override: interestOverride ?? undefined,
        penalty: penalty > 0 ? penalty : undefined,
      },
      {
        onSuccess: () => {
          toast.success(isMatured ? "Payout recorded" : "Fixed deposit closed");
          handleOpenChange(false);
          onClosed?.();
        },
        onError: (err) => {
          setError(
            err instanceof ApiClientError
              ? err.message
              : "An unexpected error occurred"
          );
        },
      }
    );
  }

  if (!fd) return null;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{isMatured ? "Record payout" : "Close early"}</DialogTitle>
          <DialogDescription>
            {isMatured
              ? "Record the maturity payout for this fixed deposit."
              : "Close this fixed deposit before its maturity date."}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          {/* Destination account */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="close-fd-destination">Destination Account</Label>
            <Select
              value={destinationAccountId || "none"}
              onValueChange={(v) =>
                setDestinationAccountId(v === "none" ? "" : v)
              }
              disabled={closeFD.isPending}
            >
              <SelectTrigger id="close-fd-destination" className="w-full">
                <SelectValue placeholder="Select account..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">No account (manual)</SelectItem>
                {cashAccounts.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.name} ({a.currency})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Summary of the deposit */}
          <div className="rounded-lg border bg-muted/40 p-4 space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Principal</span>
              <span className="font-medium">
                {formatCurrency(principal, account.currency)}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">
                {fd.payout_type === "periodic"
                  ? "Interest (already paid out)"
                  : isMatured
                    ? "Interest at maturity"
                    : "Estimated accrued interest"}
              </span>
              <span className="font-medium">
                {formatCurrency(displayInterest, account.currency)}
              </span>
            </div>
            {isEarlyClose && penalty > 0 && (
              <div className="flex justify-between text-destructive">
                <span>Penalty</span>
                <span>-{formatCurrency(penalty, account.currency)}</span>
              </div>
            )}
            <div className="flex justify-between border-t border-border/60 pt-2 font-semibold">
              <span>Amount received</span>
              <span>{formatCurrency(amountReceived, account.currency)}</span>
            </div>
          </div>

          {/* Deposit info read-only */}
          <div className="rounded-lg border bg-muted/40 p-4 space-y-2 text-sm text-muted-foreground">
            <div className="flex justify-between">
              <span>Maturity date</span>
              <span>{formatDate(fd.maturity_date)}</span>
            </div>
            <div className="flex justify-between">
              <span>Payout type</span>
              <span>{formatPayoutType(fd.payout_type)}</span>
            </div>
          </div>

          {/* Interest override */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="close-fd-interest">
              Interest Amount Override
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                (optional)
              </span>
            </Label>
            <CurrencyInput
              id="close-fd-interest"
              value={interestOverride ?? estimatedInterest}
              onChange={(v) =>
                setInterestOverride(v !== estimatedInterest ? v : null)
              }
              placeholder={String(estimatedInterest / 100)}
              disabled={closeFD.isPending}
            />
          </div>

          {/* Penalty - shown for early close only */}
          {isEarlyClose && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="close-fd-penalty">
                Early Closure Penalty
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  (optional)
                </span>
              </Label>
              <CurrencyInput
                id="close-fd-penalty"
                value={penalty}
                onChange={setPenalty}
                placeholder="0.00"
                disabled={closeFD.isPending}
              />
            </div>
          )}

          <p className="text-xs text-muted-foreground -mt-2">
            The backend recomputes the final amount authoritatively.
          </p>

          <DialogFooter>
            <Button type="submit" disabled={closeFD.isPending}>
              {closeFD.isPending
                ? "Processing..."
                : isMatured
                  ? "Record payout"
                  : "Close early"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
