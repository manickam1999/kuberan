"use client";

import { useState } from "react";
import { toast } from "sonner";
import { useAccounts } from "@/hooks/use-accounts";
import { useCreateSharedExpense } from "@/hooks/use-receivables";
import { formatCurrency, toLocalDateString, toRFC3339 } from "@/lib/format";
import {
  isSettlementAccountType,
  receivableApiErrorMessage,
} from "@/lib/receivable-format";
import {
  buildSharedCounterparties,
  normalizeCounterpartyName,
} from "@/lib/shared-expense";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CounterpartyInput } from "./counterparty-input";

export interface LendMoneyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-select the account money comes from (when opened from an account page). */
  defaultAccountId?: string;
}

/**
 * Lend money to someone. Posts a shared expense with kind `loan`, mode `full`
 * and no category, so it never reads as spending: it is money owed to you.
 */
export function LendMoneyDialog({
  open,
  onOpenChange,
  defaultAccountId,
}: LendMoneyDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {/* Inner form mounts only while open, so state resets on each open. */}
        <LendForm
          defaultAccountId={defaultAccountId}
          onDone={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

function LendForm({
  defaultAccountId,
  onDone,
}: {
  defaultAccountId?: string;
  onDone: () => void;
}) {
  const [accountId, setAccountId] = useState("");
  const [name, setName] = useState("");
  const [amount, setAmount] = useState(0);
  const [date, setDate] = useState(toLocalDateString());
  const [note, setNote] = useState("");
  const [error, setError] = useState("");

  const { data: accountData } = useAccounts({ page_size: 100 });
  const createShared = useCreateSharedExpense();

  // Same filtering as the settle dialog: cash / credit card / debt only.
  const accounts = (accountData?.data ?? [])
    .filter((a) => a.is_active && isSettlementAccountType(a.type))
    .sort((a, b) => a.name.localeCompare(b.name));
  // Prefer the requested account, then a cash account, then anything allowed.
  const defaultId =
    accounts.find((a) => a.id === defaultAccountId)?.id ??
    accounts.find((a) => a.type === "cash")?.id ??
    accounts[0]?.id ??
    "";
  const effectiveAccountId = accountId || defaultId;

  const cleanName = normalizeCounterpartyName(name);
  const summary =
    amount > 0 && cleanName
      ? `Lend ${formatCurrency(amount)} to ${cleanName}`
      : null;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!effectiveAccountId) {
      setError("Select an account to lend from");
      return;
    }
    const built = buildSharedCounterparties({
      amount,
      mode: "full",
      rows: [{ name, amount: 0 }],
    });
    if (!built.ok) {
      setError(built.error);
      return;
    }
    createShared.mutate(
      {
        account_id: effectiveAccountId,
        amount,
        description: note.trim() || `Loan to ${cleanName}`,
        date: date ? toRFC3339(date) : undefined,
        kind: "loan",
        mode: "full",
        counterparties: built.counterparties,
      },
      {
        onSuccess: () => {
          toast.success(`Lent ${formatCurrency(amount)} to ${cleanName}`);
          onDone();
        },
        onError: (err) => setError(receivableApiErrorMessage(err)),
      }
    );
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>Lend money</DialogTitle>
        <DialogDescription>
          Record money you lend to someone. It leaves your account but is not
          counted as spending, and shows up under Owed to me until it is repaid.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="lend-account">From account</Label>
        <Select
          value={effectiveAccountId}
          onValueChange={setAccountId}
          disabled={createShared.isPending}
        >
          <SelectTrigger id="lend-account" className="w-full">
            <SelectValue placeholder="Select account" />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="lend-borrower">Borrower</Label>
        <CounterpartyInput
          id="lend-borrower"
          value={name}
          onChange={setName}
          placeholder="Who are you lending to?"
          disabled={createShared.isPending}
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="lend-amount">Amount</Label>
          <CurrencyInput
            id="lend-amount"
            value={amount}
            onChange={setAmount}
            placeholder="0.00"
            disabled={createShared.isPending}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="lend-date">Date</Label>
          <Input
            id="lend-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            disabled={createShared.isPending}
          />
        </div>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="lend-note">Note (optional)</Label>
        <Input
          id="lend-note"
          value={note}
          maxLength={500}
          placeholder="What is it for?"
          onChange={(e) => setNote(e.target.value)}
          disabled={createShared.isPending}
        />
      </div>

      {summary && (
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {summary}
        </p>
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
          disabled={createShared.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={createShared.isPending}>
          {createShared.isPending ? "Saving..." : "Record loan"}
        </Button>
      </DialogFooter>
    </form>
  );
}
