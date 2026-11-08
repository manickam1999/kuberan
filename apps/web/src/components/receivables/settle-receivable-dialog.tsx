"use client";

import { useState } from "react";
import { toast } from "sonner";
import { useAccounts } from "@/hooks/use-accounts";
import { getFxAmountRequired, type FxAmountRequired } from "@/lib/api-client";
import {
  CrossCurrencyAmount,
  useCrossAmount,
} from "@/components/transactions/cross-currency-amount";
import { useSettleReceivable } from "@/hooks/use-receivables";
import { formatMoney, toLocalDateString, toRFC3339 } from "@/lib/format";
import {
  isSettlementAccountType,
  receivableApiErrorMessage,
  writeOffImpactText,
} from "@/lib/receivable-format";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { Receivable } from "@/types/models";

export interface SettleReceivableDialogProps {
  /** The receivable being repaid. Include `transaction` for the write-off impact text. */
  receivable: Receivable;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Record a repayment against a receivable. The account picker is limited to
 * cash / credit card / debt accounts and defaults to the expense's account.
 * The amount defaults to the outstanding balance (cents).
 */
export function SettleReceivableDialog({
  receivable,
  open,
  onOpenChange,
}: SettleReceivableDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-md"
        onOpenAutoFocus={(e) => {
          // Land on Amount (the usual edit), not the account select.
          e.preventDefault();
          document.getElementById("settle-amount")?.focus();
        }}
      >
        {/* Inner form mounts only while open, so state resets on each open. */}
        <SettleForm receivable={receivable} onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function SettleForm({
  receivable,
  onDone,
}: {
  receivable: Receivable;
  onDone: () => void;
}) {
  const [accountId, setAccountId] = useState("");
  const [amount, setAmount] = useState(receivable.outstanding);
  const [date, setDate] = useState(toLocalDateString());
  const [writeOffRest, setWriteOffRest] = useState(false);
  const [error, setError] = useState("");
  // Set when the API asked for account_amount although the cached accounts
  // looked like the same currency.
  const [fxRequired, setFxRequired] = useState<FxAmountRequired | null>(null);

  const { data: accountData } = useAccounts({ page_size: 100 });
  const settle = useSettleReceivable();

  const accounts = (accountData?.data ?? []).filter(
    (a) => a.is_active && isSettlementAccountType(a.type)
  );
  const expenseAccountId = receivable.transaction?.account_id;
  const defaultAccountId =
    accounts.find((a) => a.id === expenseAccountId)?.id ?? accounts[0]?.id ?? "";
  const effectiveAccountId = accountId || defaultAccountId;

  const currency = receivable.currency;
  const targetAccount = accounts.find((a) => a.id === effectiveAccountId);
  const accountCurrency =
    fxRequired?.currency || targetAccount?.currency || currency;
  // The repayment is in the receivable's currency; when the account holds
  // another one, the amount credited to it is asked for separately.
  const received = useCrossAmount(currency, accountCurrency, amount);

  const remainder = receivable.outstanding - amount;
  const impact = writeOffRest ? writeOffImpactText(receivable, remainder) : null;
  const amountValid = amount > 0 && amount <= receivable.outstanding;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!effectiveAccountId) {
      setError("Select an account");
      return;
    }
    if (!amountValid) {
      setError(
        `Amount must be between 0.01 and ${formatMoney(receivable.outstanding, currency)}`
      );
      return;
    }
    if (received.needed && received.value <= 0) {
      setError(`Enter the amount received in ${accountCurrency}`);
      return;
    }
    settle.mutate(
      {
        id: receivable.id,
        account_id: effectiveAccountId,
        amount,
        account_amount: received.needed ? received.value : undefined,
        date: date ? toRFC3339(date) : undefined,
        write_off_remainder: writeOffRest && remainder > 0 ? true : undefined,
      },
      {
        onSuccess: () => {
          toast.success("Repayment recorded");
          onDone();
        },
        onError: (err) => {
          const need = getFxAmountRequired(err);
          if (need && need.field === "account_amount") {
            setFxRequired(need);
            if (need.suggestedAmount) received.setValue(need.suggestedAmount);
            setError(
              `This account holds a different currency. Confirm the amount received in ${need.currency}.`
            );
            return;
          }
          setError(receivableApiErrorMessage(err));
        },
      }
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>Record repayment</DialogTitle>
        <DialogDescription>
          {receivable.counterparty_name} owes{" "}
          {formatMoney(receivable.outstanding, currency)}
          {receivable.transaction?.description
            ? ` for ${receivable.transaction.description}`
            : ""}
          .
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="settle-account">Received into</Label>
        <Select
          value={effectiveAccountId}
          onValueChange={(v) => {
            setAccountId(v);
            setFxRequired(null);
          }}
          disabled={settle.isPending}
        >
          <SelectTrigger id="settle-account" className="w-full">
            <SelectValue placeholder="Select account" />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name} ({a.currency})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="settle-amount">Amount</Label>
          <CurrencyInput
            id="settle-amount"
            value={amount}
            onChange={setAmount}
            symbol={currency}
            disabled={settle.isPending}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="settle-date">Date</Label>
          <Input
            id="settle-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            disabled={settle.isPending}
          />
        </div>
      </div>

      {received.needed && (
        <CrossCurrencyAmount
          id="settle-account-amount"
          label={`Amount received in ${targetAccount?.name ?? "account"} (${accountCurrency})`}
          fromCurrency={currency}
          fromAmount={amount}
          toCurrency={accountCurrency}
          value={received.value}
          onChange={received.setValue}
          rate={received.rate}
          disabled={settle.isPending}
        />
      )}

      {amountValid && remainder > 0 && (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <Checkbox
              id="settle-write-off-rest"
              checked={writeOffRest}
              onCheckedChange={(v) => setWriteOffRest(v === true)}
              disabled={settle.isPending}
            />
            <Label htmlFor="settle-write-off-rest" className="font-normal">
              Write off the rest ({formatMoney(remainder, currency)})
            </Label>
          </div>
          {impact && (
            <p className="pl-6 text-xs text-muted-foreground">{impact}</p>
          )}
        </div>
      )}

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
          disabled={settle.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={settle.isPending || !amountValid}>
          {settle.isPending ? "Saving..." : "Record repayment"}
        </Button>
      </DialogFooter>
    </form>
  );
}

