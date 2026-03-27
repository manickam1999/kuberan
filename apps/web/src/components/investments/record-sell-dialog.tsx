"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useRecordSell } from "@/hooks/use-investments";
import { useAccounts } from "@/hooks/use-accounts";
import { toRFC3339, formatCurrency } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "INSUFFICIENT_SHARES") {
      return "You do not have enough shares to sell this quantity";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

interface RecordSellDialogProps {
  investmentId: string;
  currentQuantity: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RecordSellDialog({
  investmentId,
  currentQuantity,
  open,
  onOpenChange,
}: RecordSellDialogProps) {
  const [date, setDate] = useState("");
  const [quantity, setQuantity] = useState("");
  const [pricePerUnit, setPricePerUnit] = useState(0);
  const [fee, setFee] = useState(0);
  const [notes, setNotes] = useState("");
  const [depositAccountId, setDepositAccountId] = useState("");
  const [error, setError] = useState("");

  const recordSell = useRecordSell(investmentId);
  const isSubmitting = recordSell.isPending;

  const { data: accountsData } = useAccounts({ page: 1, page_size: 100 });
  const cashAccounts =
    accountsData?.data?.filter(
      (a) => a.type === "cash" && a.is_active
    ) ?? [];

  function resetForm() {
    setDate("");
    setQuantity("");
    setPricePerUnit(0);
    setFee(0);
    setNotes("");
    setDepositAccountId("");
    setError("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (!date) {
      setError("Date is required");
      return;
    }

    const qty = parseFloat(quantity);
    if (!qty || qty <= 0) {
      setError("Quantity must be greater than zero");
      return;
    }

    if (qty > currentQuantity) {
      setError(
        `Cannot sell more than you hold (${currentQuantity.toLocaleString(undefined, { maximumFractionDigits: 6 })} units)`
      );
      return;
    }

    if (pricePerUnit <= 0) {
      setError("Price per unit must be greater than zero");
      return;
    }

    recordSell.mutate(
      {
        date: toRFC3339(date),
        quantity: qty,
        price_per_unit: pricePerUnit,
        fee: fee > 0 ? fee : undefined,
        notes: notes.trim() || undefined,
        deposit_account_id: depositAccountId && depositAccountId !== "none" ? depositAccountId : undefined,
      },
      {
        onSuccess: () => {
          toast.success("Sell recorded");
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Record Sell</DialogTitle>
          <DialogDescription>
            Record a sale of shares. You hold{" "}
            {currentQuantity.toLocaleString(undefined, {
              maximumFractionDigits: 6,
            })}{" "}
            units.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="sell-date">Date</Label>
            <Input
              id="sell-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="sell-quantity">Quantity</Label>
            <Input
              id="sell-quantity"
              type="number"
              step="any"
              min="0"
              placeholder="0"
              value={quantity}
              onChange={(e) => setQuantity(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="sell-price">Price per Unit</Label>
            <CurrencyInput
              id="sell-price"
              value={pricePerUnit}
              onChange={setPricePerUnit}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="sell-fee">Fee (optional)</Label>
            <CurrencyInput
              id="sell-fee"
              value={fee}
              onChange={setFee}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="sell-notes">Notes (optional)</Label>
            <Textarea
              id="sell-notes"
              placeholder="Optional notes..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          {cashAccounts.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label>Deposit proceeds to (optional)</Label>
              <Select
                value={depositAccountId}
                onValueChange={setDepositAccountId}
                disabled={isSubmitting}
              >
                <SelectTrigger>
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">None</SelectItem>
                  {cashAccounts.map((account) => (
                    <SelectItem key={account.id} value={account.id}>
                      {account.name} ({formatCurrency(account.balance)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {depositAccountId && depositAccountId !== "none" && pricePerUnit > 0 && quantity && (
                <p className="text-muted-foreground text-sm">
                  Net proceeds: {formatCurrency(
                    Math.round(parseFloat(quantity) * pricePerUnit) - fee
                  )}
                </p>
              )}
            </div>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Recording..." : "Record Sell"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
