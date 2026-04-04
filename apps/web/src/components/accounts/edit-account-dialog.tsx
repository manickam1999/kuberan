"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useUpdateAccount } from "@/hooks/use-accounts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { Account } from "@/types/models";
import type { UpdateAccountRequest } from "@/types/api";

const ACCOUNT_TYPE_LABELS: Record<string, string> = {
  cash: "Cash",
  investment: "Investment",
  debt: "Debt",
  credit_card: "Credit Card",
};

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (
      error.code === "INVALID_INPUT" &&
      (error.message.toLowerCase().includes("already exists") ||
        error.message.toLowerCase().includes("duplicate"))
    ) {
      return "An account with this name already exists";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

interface EditAccountDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  account: Account | null;
}

export function EditAccountDialog({
  open,
  onOpenChange,
  account,
}: EditAccountDialogProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isActive, setIsActive] = useState(true);
  const [isPinned, setIsPinned] = useState(false);
  const [broker, setBroker] = useState("");
  const [accountNumber, setAccountNumber] = useState("");
  const [interestRate, setInterestRate] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [creditLimit, setCreditLimit] = useState(0);
  const [error, setError] = useState("");

  const updateAccount = useUpdateAccount(account?.id ?? "");
  const isSubmitting = updateAccount.isPending;

  // Sync form state when account changes
  useEffect(() => {
    if (account) {
      setName(account.name);
      setDescription(account.description ?? "");
      setIsActive(account.is_active);
      setIsPinned(account.is_pinned ?? false);
      setBroker(account.broker ?? "");
      setAccountNumber(account.account_number ?? "");
      setInterestRate(account.interest_rate?.toString() ?? "");
      setDueDate(account.due_date ? account.due_date.split("T")[0] : "");
      setCreditLimit(account.credit_limit ?? 0);
      setError("");
    }
  }, [account]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!account) return;
    setError("");

    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required");
      return;
    }
    if (trimmedName.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (description.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }

    // Build payload with only changed fields
    const payload: UpdateAccountRequest = {};
    if (trimmedName !== account.name) payload.name = trimmedName;
    const newDesc = description.trim();
    if (newDesc !== (account.description ?? "")) payload.description = newDesc;
    if (isActive !== account.is_active) payload.is_active = isActive;
    if (isPinned !== (account.is_pinned ?? false)) payload.is_pinned = isPinned;

    // Investment-specific fields
    if (account.type === "investment") {
      if (broker.trim() !== (account.broker ?? ""))
        payload.broker = broker.trim();
      if (accountNumber.trim() !== (account.account_number ?? ""))
        payload.account_number = accountNumber.trim();
    }

    // Credit card-specific fields
    if (account.type === "credit_card") {
      const parsedRate = interestRate ? parseFloat(interestRate) : 0;
      if (parsedRate !== (account.interest_rate ?? 0))
        payload.interest_rate = parsedRate;
      const origDueDate = account.due_date
        ? account.due_date.split("T")[0]
        : "";
      if (dueDate !== origDueDate) payload.due_date = dueDate || undefined;
      if (creditLimit !== (account.credit_limit ?? 0))
        payload.credit_limit = creditLimit;
    }

    if (Object.keys(payload).length === 0) {
      onOpenChange(false);
      return;
    }

    updateAccount.mutate(payload, {
      onSuccess: (updated) => {
        toast.success(`Account "${updated.name}" updated`);
        onOpenChange(false);
      },
      onError: (err) => setError(getErrorMessage(err)),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Account</DialogTitle>
          <DialogDescription>Update account details.</DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label>Type</Label>
            <div className="flex items-center gap-2">
              <Badge variant="secondary">
                {ACCOUNT_TYPE_LABELS[account?.type ?? ""] ?? account?.type}
              </Badge>
              <span className="text-sm text-muted-foreground">
                {account?.currency}
              </span>
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-account-name">Name</Label>
            <Input
              id="edit-account-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={100}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-account-description">Description</Label>
            <Input
              id="edit-account-description"
              placeholder="Optional description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isSubmitting}
              maxLength={500}
            />
          </div>

          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <input
                id="edit-account-active"
                type="checkbox"
                checked={isActive}
                onChange={(e) => setIsActive(e.target.checked)}
                disabled={isSubmitting}
                className="h-4 w-4 rounded border-gray-300"
              />
              <Label htmlFor="edit-account-active">Active</Label>
            </div>
            <div className="flex items-center gap-2">
              <input
                id="edit-account-pinned"
                type="checkbox"
                checked={isPinned}
                onChange={(e) => setIsPinned(e.target.checked)}
                disabled={isSubmitting}
                className="h-4 w-4 rounded border-gray-300"
              />
              <Label htmlFor="edit-account-pinned">Pinned</Label>
            </div>
          </div>

          {account?.type === "investment" && (
            <>
              <div className="flex flex-col gap-2">
                <Label htmlFor="edit-account-broker">Broker</Label>
                <Input
                  id="edit-account-broker"
                  placeholder="e.g., Fidelity, Schwab"
                  value={broker}
                  onChange={(e) => setBroker(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="edit-account-number">Account Number</Label>
                <Input
                  id="edit-account-number"
                  placeholder="Optional"
                  value={accountNumber}
                  onChange={(e) => setAccountNumber(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={50}
                />
              </div>
            </>
          )}

          {account?.type === "credit_card" && (
            <>
              <div className="flex flex-col gap-2">
                <Label htmlFor="edit-account-credit-limit">Credit Limit</Label>
                <CurrencyInput
                  id="edit-account-credit-limit"
                  value={creditLimit}
                  onChange={setCreditLimit}
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="edit-account-interest-rate">
                  Interest Rate (%)
                </Label>
                <Input
                  id="edit-account-interest-rate"
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  placeholder="e.g., 19.99"
                  value={interestRate}
                  onChange={(e) => setInterestRate(e.target.value)}
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="edit-account-due-date">Due Date</Label>
                <Input
                  id="edit-account-due-date"
                  type="date"
                  value={dueDate}
                  onChange={(e) => setDueDate(e.target.value)}
                  disabled={isSubmitting}
                />
              </div>
            </>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving..." : "Save Changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
