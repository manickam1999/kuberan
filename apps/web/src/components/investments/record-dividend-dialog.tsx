"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError, getFxAmountRequired } from "@/lib/api-client";
import { useInvestment, useRecordDividend } from "@/hooks/use-investments";
import { useAccounts } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { toRFC3339, formatMoney } from "@/lib/format";
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

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return error.message;
  }
  return "An unexpected error occurred";
}

interface RecordDividendDialogProps {
  investmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RecordDividendDialog({
  investmentId,
  open,
  onOpenChange,
}: RecordDividendDialogProps) {
  const [date, setDate] = useState("");
  const [amount, setAmount] = useState(0);
  const [dividendType, setDividendType] = useState("cash");
  const [notes, setNotes] = useState("");
  const [fee, setFee] = useState(0);
  const [depositAccountId, setDepositAccountId] = useState("");
  const [fxRate, setFxRate] = useState("");
  const [error, setError] = useState("");

  const recordDividend = useRecordDividend(investmentId);
  const isSubmitting = recordDividend.isPending;

  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const { data: investment } = useInvestment(investmentId);
  const securityCurrency = investment?.currency || baseCurrency;
  const { data: accountsData } = useAccounts({ page: 1, page_size: 100 });
  const cashAccounts =
    accountsData?.data?.filter((a) => a.type === "cash" && a.is_active) ?? [];
  const selectedAccount = cashAccounts.find((a) => a.id === depositAccountId);
  // The net dividend (gross minus withholding/fees), in the security currency.
  const netAmount = Math.max(amount - fee, 0);
  const cross = useCrossCurrencyAmount({
    securityCurrency,
    account: selectedAccount,
    nativeTotal: netAmount,
  });

  function resetForm() {
    setDate("");
    setAmount(0);
    setDividendType("cash");
    setNotes("");
    setFee(0);
    setDepositAccountId("");
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

    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }

    if (fee > amount) {
      setError("Withholding / fees cannot exceed the dividend amount");
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

    recordDividend.mutate(
      {
        date: toRFC3339(date),
        amount,
        fee: fee > 0 ? fee : undefined,
        deposit_account_id:
          depositAccountId && depositAccountId !== "none"
            ? depositAccountId
            : undefined,
        deposit_amount: cross.needed ? cross.value : undefined,
        fx_rate: fxRate.trim() || undefined,
        dividend_type: dividendType || undefined,
        notes: notes.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.success("Dividend recorded");
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
          <DialogTitle>Record Dividend</DialogTitle>
          <DialogDescription>
            Record a dividend payment for this investment.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="div-date">Date</Label>
            <Input
              id="div-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="div-amount">Gross amount</Label>
            <CurrencyInput
              id="div-amount"
              value={amount}
              onChange={setAmount}
              currency={securityCurrency}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="div-fee">Withholding / fees (optional)</Label>
            <CurrencyInput
              id="div-fee"
              value={fee}
              onChange={setFee}
              currency={securityCurrency}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="div-type">Dividend Type</Label>
            <Select
              value={dividendType}
              onValueChange={setDividendType}
              disabled={isSubmitting}
            >
              <SelectTrigger id="div-type">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="cash">Cash</SelectItem>
                <SelectItem value="stock">Stock</SelectItem>
                <SelectItem value="special">Special</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="div-notes">Notes (optional)</Label>
            <Textarea
              id="div-notes"
              placeholder="Optional notes..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          {cashAccounts.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label>Deposit to account (optional)</Label>
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
                      {account.name} (
                      {formatMoney(account.balance, account.currency, hideBalances)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {netAmount > 0 && (
                <p className="text-muted-foreground text-sm">
                  Net dividend:{" "}
                  {formatMoney(netAmount, securityCurrency, hideBalances)}
                </p>
              )}
            </div>
          )}

          <CrossCurrencyAmountField
            id="div-account-amount"
            label={`Amount received in ${selectedAccount?.name ?? "account"} (${cross.currency})`}
            state={cross}
            disabled={isSubmitting}
          />

          <FxRateField
            id="div-fx-rate"
            securityCurrency={securityCurrency}
            value={fxRate}
            onChange={setFxRate}
            disabled={isSubmitting}
          />

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Recording..." : "Record Dividend"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
