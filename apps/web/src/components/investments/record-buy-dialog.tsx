"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError, getFxAmountRequired } from "@/lib/api-client";
import { useRecordBuy } from "@/hooks/use-investments";
import { useAccounts } from "@/hooks/use-accounts";
import { useInvestment } from "@/hooks/use-investments";
import { toRFC3339, formatMoney } from "@/lib/format";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import {
  CrossCurrencyAmountField,
  FxRateField,
  isValidFxRate,
  useCrossCurrencyAmount,
} from "@/components/investments/cross-currency-field";
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
  const [fxRate, setFxRate] = useState("");
  const [error, setError] = useState("");

  const recordBuy = useRecordBuy(investmentId);
  const isSubmitting = recordBuy.isPending;

  const { data: accountsData } = useAccounts({ page: 1, page_size: 100 });
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const { data: investment } = useInvestment(investmentId);
  const securityCurrency = investment?.currency || baseCurrency;
  const cashAccounts =
    accountsData?.data?.filter(
      (a) => a.type === "cash" && a.is_active
    ) ?? [];

  const selectedAccount = cashAccounts.find((a) => a.id === fundingAccountId);
  const qtyNum = parseFloat(quantity);
  const nativeTotal =
    qtyNum > 0 && pricePerUnit > 0
      ? Math.round(qtyNum * pricePerUnit) + fee
      : 0;
  const cross = useCrossCurrencyAmount({
    securityCurrency,
    account: selectedAccount,
    nativeTotal,
  });

  function resetForm() {
    setDate("");
    setQuantity("");
    setPricePerUnit(0);
    setFee(0);
    setNotes("");
    setFundingAccountId("");
    setFxRate("");
    cross.reset();
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

    if (!isValidFxRate(fxRate)) {
      setError("Exchange rate must be a positive number");
      return;
    }

    if (cross.needed && cross.value <= 0) {
      setError(`Enter the amount in ${cross.currency}`);
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
        funding_amount: cross.needed ? cross.value : undefined,
        fx_rate: fxRate.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.success("Buy recorded");
          handleOpenChange(false);
        },
        onError: (err) => {
          const required = getFxAmountRequired(err);
          if (required) cross.applyHint(required);
          setError(getErrorMessage(err));
        },
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
              currency={securityCurrency}
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
              currency={securityCurrency}
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
                      {account.name} ({formatMoney(account.balance, account.currency, hideBalances)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {nativeTotal > 0 && (
                <p className="text-muted-foreground text-sm">
                  Total cost: {formatMoney(nativeTotal, securityCurrency, hideBalances)}
                </p>
              )}
            </div>
          )}

          <CrossCurrencyAmountField
            id="buy-account-amount"
            label={`Amount charged to ${selectedAccount?.name ?? "account"} (${cross.currency})`}
            state={cross}
            disabled={isSubmitting}
          />

          <FxRateField
            id="buy-fx-rate"
            securityCurrency={securityCurrency}
            value={fxRate}
            onChange={setFxRate}
            disabled={isSubmitting}
          />

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
