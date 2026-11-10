"use client";

import { useState } from "react";
import { BudgetAverageHint } from "./budget-average-hint";
import { CategoryIcon } from "@/components/categories/category-icon";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useCreateBudget } from "@/hooks/use-budgets";
import { useCategories } from "@/hooks/use-categories";
import { useBaseCurrency } from "@/hooks/use-profile";
import { REPORTING_CURRENCIES } from "@/lib/currencies";
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
import type { BudgetPeriod } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "BUDGET_ALREADY_EXISTS") {
      return "An active budget already exists for this category and period.";
    }
    if (error.code === "CATEGORY_NOT_FOUND") {
      return "The selected category no longer exists.";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

interface CreateBudgetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateBudgetDialog({
  open,
  onOpenChange,
}: CreateBudgetDialogProps) {
  const [name, setName] = useState("");
  const [categoryId, setCategoryId] = useState("");
  // null until the user picks: the budget then follows the reporting currency.
  const [currency, setCurrency] = useState<string | null>(null);
  const [amount, setAmount] = useState(0);
  const [period, setPeriod] = useState<BudgetPeriod>("monthly");
  const [error, setError] = useState("");

  const baseCurrency = useBaseCurrency();
  const selectedCurrency = currency ?? baseCurrency;
  const currencyOptions = REPORTING_CURRENCIES.some(
    (c) => c.code === selectedCurrency
  )
    ? REPORTING_CURRENCIES
    : [
        { code: selectedCurrency, name: selectedCurrency },
        ...REPORTING_CURRENCIES,
      ];

  const createBudget = useCreateBudget();
  const isSubmitting = createBudget.isPending;

  // Budgets only make sense on expense categories (D6).
  const { data: categoryData } = useCategories({
    type: "expense",
    page_size: 100,
  });
  const categoryOptions = (categoryData?.data ?? [])
    .slice()
    .sort((a, b) => a.name.localeCompare(b.name));

  function resetForm() {
    setName("");
    setCategoryId("");
    setCurrency(null);
    setAmount(0);
    setPeriod("monthly");
    setError("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) resetForm();
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
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
    if (!categoryId) {
      setError("Category is required");
      return;
    }
    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }

    createBudget.mutate(
      {
        name: trimmedName,
        category_id: categoryId,
        amount,
        period,
        currency: selectedCurrency,
      },
      {
        onSuccess: (budget) => {
          toast.success(`Budget "${budget.name}" created`);
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
          <DialogTitle>Create Budget</DialogTitle>
          <DialogDescription>
            Set a recurring spending cap for an expense category.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="budget-name">Name</Label>
            <Input
              id="budget-name"
              placeholder="e.g. Monthly groceries"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={100}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="budget-category">Category</Label>
            <Select
              value={categoryId}
              onValueChange={setCategoryId}
              disabled={isSubmitting}
            >
              <SelectTrigger id="budget-category" className="w-full">
                <SelectValue placeholder="Select expense category" />
              </SelectTrigger>
              <SelectContent>
                {categoryOptions.length === 0 ? (
                  <SelectItem value="none" disabled>
                    No expense categories
                  </SelectItem>
                ) : (
                  categoryOptions.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      <span className="flex items-center gap-2">
                        {c.icon && <CategoryIcon icon={c.icon} name={c.name} />}
                        {c.name}
                      </span>
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="budget-currency">Currency</Label>
            <Select
              value={selectedCurrency}
              onValueChange={setCurrency}
              disabled={isSubmitting}
            >
              <SelectTrigger id="budget-currency" className="w-full">
                <SelectValue />
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

          <div className="flex flex-col gap-2">
            <Label htmlFor="budget-amount">Amount</Label>
            <CurrencyInput
              id="budget-amount"
              currency={selectedCurrency}
              value={amount}
              onChange={setAmount}
              disabled={isSubmitting}
            />
            {categoryId && (
              <BudgetAverageHint
                categoryId={categoryId}
                categoryName={
                  categoryOptions.find((c) => c.id === categoryId)?.name ??
                  "this category"
                }
                currency={selectedCurrency}
              />
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="budget-period">Period</Label>
            <Select
              value={period}
              onValueChange={(v) => setPeriod(v as BudgetPeriod)}
              disabled={isSubmitting}
            >
              <SelectTrigger id="budget-period" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="monthly">Monthly</SelectItem>
                <SelectItem value="yearly">Yearly</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Creating..." : "Create Budget"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
