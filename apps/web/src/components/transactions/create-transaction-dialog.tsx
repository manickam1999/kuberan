"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ArrowDownRight, ArrowUpRight, ArrowLeftRight } from "lucide-react";
import { ApiClientError } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { useAccounts } from "@/hooks/use-accounts";
import { useCreateTransaction, useCreateTransfer } from "@/hooks/use-transactions";
import { useCategories } from "@/hooks/use-categories";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { toRFC3339 } from "@/lib/format";
import type { TransactionType } from "@/types/models";

type DialogType = TransactionType | "transfer";

const TYPE_OPTIONS: {
  value: DialogType;
  label: string;
  icon: typeof ArrowUpRight;
  activeText: string;
}[] = [
  {
    value: "expense",
    label: "Expense",
    icon: ArrowDownRight,
    activeText: "text-negative",
  },
  {
    value: "income",
    label: "Income",
    icon: ArrowUpRight,
    activeText: "text-positive",
  },
  {
    value: "transfer",
    label: "Transfer",
    icon: ArrowLeftRight,
    activeText: "text-info",
  },
];

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "SAME_ACCOUNT") {
      return "Cannot transfer to the same account";
    }
    if (error.code === "INSUFFICIENT_BALANCE") {
      return "Insufficient balance for this transfer";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

function todayISO(): string {
  return new Date().toISOString().split("T")[0];
}

interface CreateTransactionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-select an account when opening from account detail page */
  defaultAccountId?: string;
}

export function CreateTransactionDialog({
  open,
  onOpenChange,
  defaultAccountId,
}: CreateTransactionDialogProps) {
  const [accountId, setAccountId] = useState<string>(defaultAccountId ?? "");
  const [type, setType] = useState<DialogType>("expense");
  const [amount, setAmount] = useState(0);
  const [categoryId, setCategoryId] = useState<string>("");
  const [description, setDescription] = useState("");
  const [date, setDate] = useState(todayISO());
  const [error, setError] = useState("");

  // Transfer-specific state
  const [fromAccountId, setFromAccountId] = useState<string>(
    defaultAccountId ?? ""
  );
  const [toAccountId, setToAccountId] = useState<string>("");

  const createTransaction = useCreateTransaction();
  const createTransfer = useCreateTransfer();
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const { data: categoriesData } = useCategories({
    page_size: 100,
    type: type === "income" ? "income" : "expense",
  });

  const accounts = [...(accountsData?.data ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name)
  );
  const categories = [...(categoriesData?.data ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name)
  );
  const isTransfer = type === "transfer";
  const isSubmitting = createTransaction.isPending || createTransfer.isPending;

  // For transfer: filter to accounts, exclude selected from-account for to-account
  const toAccounts = accounts.filter(
    (a) => a.is_active && a.id !== fromAccountId
  );

  function resetForm() {
    setAccountId(defaultAccountId ?? "");
    setType("expense");
    setAmount(0);
    setCategoryId("");
    setDescription("");
    setDate(todayISO());
    setError("");
    setFromAccountId(defaultAccountId ?? "");
    setToAccountId("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  function handleTypeChange(newType: DialogType) {
    setType(newType);
    setCategoryId("");
    if (newType === "transfer") {
      setFromAccountId(accountId || defaultAccountId || "");
    } else {
      if (type === "transfer" && fromAccountId) {
        setAccountId(fromAccountId);
      }
    }
  }

  function handleFromAccountChange(value: string) {
    setFromAccountId(value);
    if (toAccountId === value) {
      setToAccountId("");
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }

    if (isTransfer) {
      if (!fromAccountId) {
        setError("Please select a source account");
        return;
      }
      if (!toAccountId) {
        setError("Please select a destination account");
        return;
      }
      if (fromAccountId === toAccountId) {
        setError("Cannot transfer to the same account");
        return;
      }

      createTransfer.mutate(
        {
          from_account_id: fromAccountId,
          to_account_id: toAccountId,
          amount,
          description: description.trim() || undefined,
          date: date ? toRFC3339(date) : undefined,
        },
        {
          onSuccess: () => {
            toast.success("Transfer completed");
            handleOpenChange(false);
          },
          onError: (err) => setError(getErrorMessage(err)),
        }
      );
    } else {
      if (!accountId) {
        setError("Please select an account");
        return;
      }

      createTransaction.mutate(
        {
          account_id: accountId,
          type: type as TransactionType,
          amount,
          category_id:
            categoryId && categoryId !== "none" ? categoryId : undefined,
          description: description.trim() || undefined,
          date: date ? toRFC3339(date) : undefined,
        },
        {
          onSuccess: () => {
            toast.success("Transaction created");
            handleOpenChange(false);
          },
          onError: (err) => setError(getErrorMessage(err)),
        }
      );
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isTransfer ? "Transfer funds" : "New transaction"}
          </DialogTitle>
          <DialogDescription>
            {isTransfer
              ? "Move money between your accounts."
              : "Record an income or expense."}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Type segmented control */}
          <div className="grid grid-cols-3 gap-1 rounded-xl bg-muted p-1">
            {TYPE_OPTIONS.map((opt) => {
              const active = type === opt.value;
              const Icon = opt.icon;
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => handleTypeChange(opt.value)}
                  disabled={isSubmitting}
                  className={cn(
                    "flex items-center justify-center gap-1.5 rounded-lg px-2 py-2 text-sm font-medium transition-all disabled:opacity-50",
                    active
                      ? cn("bg-background shadow-sm", opt.activeText)
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <Icon className="size-4" />
                  {opt.label}
                </button>
              );
            })}
          </div>

          {/* Amount — the primary field */}
          <div className="flex flex-col gap-1.5">
            <Label
              htmlFor="tx-amount"
              className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
            >
              Amount
            </Label>
            <CurrencyInput
              id="tx-amount"
              value={amount}
              onChange={setAmount}
              symbol="MYR"
              placeholder="0.00"
              disabled={isSubmitting}
              className="money h-14 pl-16 text-2xl font-semibold"
            />
          </div>

          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          {/* Account (expense/income) */}
          {!isTransfer && !defaultAccountId && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="tx-account">Account</Label>
              <Select
                value={accountId}
                onValueChange={setAccountId}
                disabled={isSubmitting}
              >
                <SelectTrigger id="tx-account" className="w-full">
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
          )}

          {/* From / To (transfer) */}
          {isTransfer && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="tx-from-account">From</Label>
                <Select
                  value={fromAccountId}
                  onValueChange={handleFromAccountChange}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="tx-from-account" className="w-full">
                    <SelectValue placeholder="Source" />
                  </SelectTrigger>
                  <SelectContent>
                    {accounts
                      .filter((a) => a.is_active)
                      .map((a) => (
                        <SelectItem key={a.id} value={a.id}>
                          {a.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="tx-to-account">To</Label>
                <Select
                  value={toAccountId}
                  onValueChange={setToAccountId}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="tx-to-account" className="w-full">
                    <SelectValue placeholder="Destination" />
                  </SelectTrigger>
                  <SelectContent>
                    {toAccounts.map((a) => (
                      <SelectItem key={a.id} value={a.id}>
                        {a.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          {/* Category (expense/income) */}
          {!isTransfer && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="tx-category">Category</Label>
              <Select
                value={categoryId}
                onValueChange={setCategoryId}
                disabled={isSubmitting}
              >
                <SelectTrigger id="tx-category" className="w-full">
                  <SelectValue placeholder="Select category (optional)" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No category</SelectItem>
                  {categories.map((cat) => (
                    <SelectItem key={cat.id} value={cat.id}>
                      {cat.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Date + Description */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="tx-date">Date</Label>
            <Input
              id="tx-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="tx-description">Note</Label>
            <Input
              id="tx-description"
              placeholder="Optional description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <DialogFooter className="mt-1">
            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting
                ? isTransfer
                  ? "Transferring…"
                  : "Adding…"
                : isTransfer
                  ? "Transfer funds"
                  : "Add transaction"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
