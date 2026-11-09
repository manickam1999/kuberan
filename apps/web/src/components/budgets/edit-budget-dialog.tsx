"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useUpdateBudget } from "@/hooks/use-budgets";
import { REPORTING_CURRENCIES } from "@/lib/currencies";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Segmented } from "@/components/ui/segmented";
import { Pill } from "@/components/ui/pill";
import { BudgetAverageHint } from "./budget-average-hint";
import { Pause, Play, Trash2 } from "lucide-react";
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
  /** Asks the page to open the delete confirmation for this budget. */
  onDelete?: (budget: Budget) => void;
}

export function EditBudgetDialog({
  open,
  onOpenChange,
  budget,
  onDelete,
}: EditBudgetDialogProps) {
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState("");
  const [amount, setAmount] = useState(0);
  const [period, setPeriod] = useState<BudgetPeriod>("monthly");
  const [error, setError] = useState("");

  const updateBudget = useUpdateBudget(budget?.id ?? "");
  const isSubmitting = updateBudget.isPending;

  const selectedCurrency = currency || budget?.currency || "";
  const currencyOptions =
    selectedCurrency === "" ||
    REPORTING_CURRENCIES.some((c) => c.code === selectedCurrency)
      ? REPORTING_CURRENCIES
      : [
          { code: selectedCurrency, name: selectedCurrency },
          ...REPORTING_CURRENCIES,
        ];

  useEffect(() => {
    if (budget) {
      setName(budget.name);
      setCurrency(budget.currency);
      setAmount(budget.amount);
      setPeriod(budget.period);
      setError("");
    }
  }, [budget]);

  function togglePaused() {
    if (!budget) return;
    updateBudget.mutate(
      { is_active: !budget.is_active },
      {
        onSuccess: () => {
          toast.success(
            budget.is_active
              ? `Budget "${budget.name}" paused`
              : `Budget "${budget.name}" resumed`
          );
          onOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

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
    if (selectedCurrency !== budget.currency) payload.currency = selectedCurrency;
    if (period !== budget.period) payload.period = period;

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

  const categoryName = budget?.category?.name ?? "this category";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {budget?.category?.name ? `${budget.category.name} budget` : "Edit budget"}
          </DialogTitle>
          <DialogDescription>
            {budget && !budget.is_active
              ? "This budget is paused and left out of progress."
              : "Change the cap, the currency or how often it repeats."}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label>Category</Label>
              <div>
                <Pill color={budget?.category?.color || undefined} className="h-9 px-3 text-sm">
                  {categoryName}
                </Pill>
              </div>
            </div>
            <div className="flex flex-col gap-2">
              <Label id="edit-budget-period-label">Repeats</Label>
              <Segmented
                aria-label="Repeats"
                full
                value={period}
                onValueChange={(v) => setPeriod(v as BudgetPeriod)}
                options={[
                  { value: "monthly", label: "Monthly" },
                  { value: "yearly", label: "Yearly" },
                ]}
              />
            </div>
          </div>

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

          <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
            <div className="flex flex-col gap-2">
              <Label htmlFor="edit-budget-amount">Amount</Label>
              <CurrencyInput
                id="edit-budget-amount"
                currency={selectedCurrency}
                value={amount}
                onChange={setAmount}
                disabled={isSubmitting}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="edit-budget-currency">Currency</Label>
              <Select
                value={selectedCurrency}
                onValueChange={setCurrency}
                disabled={isSubmitting}
              >
                <SelectTrigger id="edit-budget-currency" className="w-full">
                  <SelectValue>{selectedCurrency}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {currencyOptions.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      {c.code} · {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          {budget && (
            <BudgetAverageHint
              categoryId={budget.category_id}
              categoryName={categoryName}
              currency={selectedCurrency}
            />
          )}

          <DialogFooter className="sm:justify-between">
            <div className="flex gap-1">
              <Button
                type="button"
                variant="ghost"
                onClick={togglePaused}
                disabled={isSubmitting}
              >
                {budget?.is_active === false ? (
                  <Play className="size-4" />
                ) : (
                  <Pause className="size-4" />
                )}
                {budget?.is_active === false ? "Resume" : "Pause"}
              </Button>
              {onDelete && budget && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="text-muted-foreground hover:text-destructive"
                  aria-label="Delete budget"
                  disabled={isSubmitting}
                  onClick={() => onDelete(budget)}
                >
                  <Trash2 className="size-4" />
                </Button>
              )}
            </div>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
