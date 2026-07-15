"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useUpdateBudget } from "@/hooks/use-budgets";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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
import type { Budget, BudgetPeriod } from "@/types/models";
import type { UpdateBudgetRequest } from "@/types/api";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "BUDGET_ALREADY_EXISTS") {
      return "An active budget already exists for this category and period.";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

interface EditBudgetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  budget: Budget | null;
}

export function EditBudgetDialog({
  open,
  onOpenChange,
  budget,
}: EditBudgetDialogProps) {
  const [name, setName] = useState("");
  const [amount, setAmount] = useState(0);
  const [period, setPeriod] = useState<BudgetPeriod>("monthly");
  const [isActive, setIsActive] = useState(true);
  const [error, setError] = useState("");

  const updateBudget = useUpdateBudget(budget?.id ?? "");
  const isSubmitting = updateBudget.isPending;

  useEffect(() => {
    if (budget) {
      setName(budget.name);
      setAmount(budget.amount);
      setPeriod(budget.period);
      setIsActive(budget.is_active);
      setError("");
    }
  }, [budget]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!budget) return;
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
    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }

    const payload: UpdateBudgetRequest = {};
    if (trimmedName !== budget.name) payload.name = trimmedName;
    if (amount !== budget.amount) payload.amount = amount;
    if (period !== budget.period) payload.period = period;
    if (isActive !== budget.is_active) payload.is_active = isActive;

    if (Object.keys(payload).length === 0) {
      onOpenChange(false);
      return;
    }

    updateBudget.mutate(payload, {
      onSuccess: (updated) => {
        toast.success(`Budget "${updated.name}" updated`);
        onOpenChange(false);
      },
      onError: (err) => setError(getErrorMessage(err)),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Budget</DialogTitle>
          <DialogDescription>Update this budget&apos;s details.</DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-budget-name">Name</Label>
            <Input
              id="edit-budget-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={100}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-budget-amount">Amount</Label>
            <CurrencyInput
              id="edit-budget-amount"
              value={amount}
              onChange={setAmount}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-budget-period">Period</Label>
            <Select
              value={period}
              onValueChange={(v) => setPeriod(v as BudgetPeriod)}
              disabled={isSubmitting}
            >
              <SelectTrigger id="edit-budget-period" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="monthly">Monthly</SelectItem>
                <SelectItem value="yearly">Yearly</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="flex flex-col gap-0.5">
              <Label htmlFor="edit-budget-active">Active</Label>
              <span className="text-xs text-muted-foreground">
                Paused budgets are excluded from progress rollups.
              </span>
            </div>
            <Switch
              id="edit-budget-active"
              checked={isActive}
              onCheckedChange={setIsActive}
              disabled={isSubmitting}
            />
          </div>

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
