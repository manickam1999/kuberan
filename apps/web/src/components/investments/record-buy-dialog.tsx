"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useRecordBuy } from "@/hooks/use-investments";
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
    return error.message;
  }
  return "An unexpected error occurred";
}

interface RecordBuyDialogProps {
  investmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RecordBuyDialog({
  investmentId,
  open,
  onOpenChange,
}: RecordBuyDialogProps) {
  const [date, setDate] = useState("");
  const [quantity, setQuantity] = useState("");
  const [pricePerUnit, setPricePerUnit] = useState(0);
  const [fee, setFee] = useState(0);
  const [notes, setNotes] = useState("");
  const [fundingAccountId, setFundingAccountId] = useState("");
  const [error, setError] = useState("");

  const recordBuy = useRecordBuy(investmentId);
  const isSubmitting = recordBuy.isPending;

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
    setFundingAccountId("");
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

    if (pricePerUnit <= 0) {
      setError("Price per unit must be greater than zero");
      return;
    }

    recordBuy.mutate(
      {
        date: toRFC3339(date),
        quantity: qty,
        price_per_unit: pricePerUnit,
        fee: fee > 0 ? fee : undefined,
        notes: notes.trim() || undefined,
        funding_account_id: fundingAccountId && fundingAccountId !== "none" ? fundingAccountId : undefined,
      },
      {
        onSuccess: () => {
          toast.success("Buy recorded");
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
          <DialogTitle>Record Buy</DialogTitle>
          <DialogDescription>
            Record a purchase of additional shares.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="buy-date">Date</Label>
            <Input
              id="buy-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="buy-quantity">Quantity</Label>
            <Input
              id="buy-quantity"
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
            <Label htmlFor="buy-price">Price per Unit</Label>
            <CurrencyInput
              id="buy-price"
              value={pricePerUnit}
              onChange={setPricePerUnit}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="buy-fee">Fee (optional)</Label>
            <CurrencyInput
              id="buy-fee"
              value={fee}
              onChange={setFee}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="buy-notes">Notes (optional)</Label>
            <Textarea
              id="buy-notes"
              placeholder="Optional notes..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          {cashAccounts.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label>Deduct from account (optional)</Label>
              <Select
                value={fundingAccountId}
                onValueChange={setFundingAccountId}
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
              {fundingAccountId && fundingAccountId !== "none" && pricePerUnit > 0 && quantity && (
                <p className="text-muted-foreground text-sm">
                  Total cost: {formatCurrency(
                    Math.round(parseFloat(quantity) * pricePerUnit) + fee
                  )}
                </p>
              )}
            </div>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Recording..." : "Record Buy"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
