"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Filter } from "lucide-react";
import { ApiClientError } from "@/lib/api-client";
import { RuleDialog } from "@/components/rules/rule-dialog";
import { suggestConditionValue } from "@/lib/rule-format";
import { cn } from "@/lib/utils";
import { useBaseCurrency } from "@/hooks/use-profile";
import { TransactionAmount } from "@/components/transactions/transaction-amount";
import { useAccounts } from "@/hooks/use-accounts";
import {
  useUpdateTransaction,
  useDeleteTransaction,
  useTransaction,
} from "@/hooks/use-transactions";
import { ReimbursementView } from "@/components/transactions/reimbursement-view";
import { toReceivableExpense } from "@/lib/shared-expense";
import { ReceivableList } from "@/components/receivables/receivable-list";
import { ReceivablesEditor } from "@/components/receivables/receivables-editor";
import { receivableErrorMessage } from "@/lib/receivable-format";
import { useCategories } from "@/hooks/use-categories";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Segmented } from "@/components/ui/segmented";
import { Textarea } from "@/components/ui/textarea";
import { Tile } from "@/components/ui/tile";
import { Money } from "@/components/ui/money";
import { transactionVisual } from "@/lib/domain-visuals";
import { CategoryTile } from "@/components/transactions/category-picker";
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
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { ExistingAttachments } from "@/components/transactions/transaction-attachments";
import { formatMoney, formatDate, toRFC3339 } from "@/lib/format";
import type { Transaction, TransactionType } from "@/types/models";
import type { RuleConditionInput, UpdateTransactionRequest } from "@/types/api";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    const friendly = receivableErrorMessage(error.code);
    if (friendly) return friendly;
    switch (error.code) {
      case "ACCOUNT_NOT_FOUND":
        return "Selected account not found";
      default:
        return error.message;
    }
  }
  return "An unexpected error occurred";
}

const TYPE_CHANGE_BLOCKED =
  "The type can't be changed while money is owed back on this expense.";

interface EditTransactionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  transaction: Transaction | null;
}

export function EditTransactionDialog({
  open,
  onOpenChange,
  transaction,
}: EditTransactionDialogProps) {
  const [type, setType] = useState<TransactionType>("expense");
  const [accountId, setAccountId] = useState<string>("");
  const [amount, setAmount] = useState(0);
  const [categoryId, setCategoryId] = useState<string>("");
  const [description, setDescription] = useState("");
  const [notes, setNotes] = useState("");
  const [date, setDate] = useState("");
  const [error, setError] = useState("");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [ruleOpen, setRuleOpen] = useState(false);
  const [ruleSeed, setRuleSeed] = useState<{
    conditions: RuleConditionInput[];
    categoryId?: string;
  }>({ conditions: [] });

  const updateTransaction = useUpdateTransaction(transaction?.id ?? "");
  const deleteTransaction = useDeleteTransaction();
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
  const isSaving = updateTransaction.isPending;
  const isDeleting = deleteTransaction.isPending;
  const isEditable =
    transaction?.type === "income" ||
    transaction?.type === "expense" ||
    transaction?.type === "opening_balance";
  const isReimbursement = transaction?.type === "reimbursement";
  // Opening balances keep their account and are never categorised; only the
  // amount, date and note can change.
  const isOpeningBalance = transaction?.type === "opening_balance";

  // The list row may be stale after receivables are edited or repaid; the
  // detail query is invalidated by those mutations, so prefer it.
  const { data: freshTransaction } = useTransaction(
    open && transaction ? transaction.id : ""
  );
  const live =
    freshTransaction && freshTransaction.id === transaction?.id
      ? freshTransaction
      : transaction;
  // The detail query may not preload `account`; fall back to the list row,
  // then to the accounts query, and never leave the value empty.
  const accountName =
    transaction?.account?.name ||
    live?.account?.name ||
    accounts.find((a) => a.id === transaction?.account_id)?.name ||
    "Unknown account";
  // The transaction's currency is its account's; the amount input follows the
  // account picked in the form.
  const baseCurrency = useBaseCurrency();
  const txCurrency =
    accounts.find((a) => a.id === live?.account_id)?.currency ??
    live?.account?.currency ??
    baseCurrency;
  const inputCurrency =
    accounts.find((a) => a.id === accountId)?.currency ?? txCurrency;
  const via =
    live?.source === "mcp" ? live.source_client_name || "an app" : null;
  const headerVisual = transactionVisual(live?.type ?? "expense");
  const headerCategory =
    categories.find((c) => c.id === (categoryId === "none" ? "" : categoryId)) ??
    live?.category ??
    null;
  const receivables = live?.receivables ?? [];
  const isShared = live?.type === "expense" && receivables.length > 0;
  // Remount the editor when the saved receivables change.
  const editorKey = receivables.map((r) => `${r.id}@${r.updated_at}`).join("|");
  const receivablesTotal = receivables.reduce((s, r) => s + r.amount, 0);
  const sharedPersonal =
    live?.personal_amount ?? (live ? live.amount - receivablesTotal : 0);

  // Sync form state from the live record (the refreshed detail once loaded,
  // the list row until then). Only re-sync when the record actually changed,
  // so a background refetch never wipes what the user is typing.
  const syncSignature = live
    ? [
        live.id,
        live.updated_at,
        live.type,
        live.account_id,
        live.amount,
        live.category_id ?? "",
        live.description ?? "",
        live.notes ?? "",
        live.date ?? "",
      ].join("|")
    : "";
  useEffect(() => {
    if (live) {
      setType(live.type);
      setAccountId(live.account_id);
      setAmount(live.amount);
      setCategoryId(live.category_id ?? "none");
      setDescription(live.description ?? "");
      setNotes(live.notes ?? "");
      setDate(live.date ? live.date.split("T")[0] : "");
      setError("");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- syncSignature captures every field read from `live`
  }, [syncSignature]);

  function handleTypeChange(newType: TransactionType) {
    setType(newType);
    setCategoryId("none");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!live) return;
    setError("");

    if (!accountId) {
      setError("Please select an account");
      return;
    }
    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }
    if (isShared && type !== "expense") {
      setError(TYPE_CHANGE_BLOCKED);
      return;
    }
    if (isShared && amount < receivablesTotal) {
      setError(
        `Amount can't be less than the ${formatMoney(receivablesTotal, txCurrency)} others owe you. Edit the money owed back first.`
      );
      return;
    }

    // Build payload with only changed fields
    const payload: UpdateTransactionRequest = {};

    if (accountId !== live.account_id)
      payload.account_id = accountId;
    if (type !== live.type) payload.type = type;
    if (amount !== live.amount) payload.amount = amount;

    // Category: compare new value with original
    const newCatId =
      categoryId && categoryId !== "none" ? categoryId : null;
    const origCatId = live.category_id ?? null;
    if (newCatId !== origCatId) payload.category_id = newCatId;

    const trimmedDesc = description.trim();
    if (trimmedDesc !== (live.description ?? ""))
      payload.description = trimmedDesc;

    if (notes.trim() !== (live.notes ?? "")) payload.notes = notes.trim();

    const origDate = live.date ? live.date.split("T")[0] : "";
    if (date !== origDate) payload.date = date ? toRFC3339(date) : undefined;

    if (Object.keys(payload).length === 0) {
      onOpenChange(false);
      return;
    }

    updateTransaction.mutate(payload, {
      onSuccess: () => {
        toast.success("Transaction updated");
        onOpenChange(false);
      },
      onError: (err) => setError(getErrorMessage(err)),
    });
  }

  // Open the rule builder prefilled from this transaction (an editable
  // "description contains <token>" condition + the current category).
  function handleCreateRule() {
    const token = suggestConditionValue(description);
    setRuleSeed({
      conditions: [
        { field: "description", operator: "contains", value_text: token },
      ],
      categoryId: categoryId && categoryId !== "none" ? categoryId : undefined,
    });
    onOpenChange(false);
    setRuleOpen(true);
  }

  function handleDelete() {
    if (!transaction) return;
    deleteTransaction.mutate(transaction.id, {
      onSuccess: () => {
        toast.success("Transaction deleted");
        setDeleteOpen(false);
        onOpenChange(false);
      },
      onError: (err) => {
        setDeleteOpen(false);
        toast.error(
          err instanceof ApiClientError
            ? (receivableErrorMessage(err.code) ?? err.message)
            : "Failed to delete transaction"
        );
      },
    });
  }

  const title =
    live?.description ||
    (isReimbursement
      ? "Repayment"
      : isOpeningBalance
        ? "Opening balance"
        : headerVisual.label);
  const canSuggestRule =
    isEditable &&
    !isOpeningBalance &&
    !!description.trim() &&
    !!categoryId &&
    categoryId !== "none" &&
    categoryId === (live?.category_id ?? "");
  const suggestedCategory = categories.find((c) => c.id === categoryId);

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent
          className="sm:max-w-lg"
          onOpenAutoFocus={(e) => {
            // On touch devices don't pop the keyboard up over the sheet.
            if (window.matchMedia("(pointer: coarse)").matches) e.preventDefault();
          }}
          onKeyDown={(e) => {
            // Cmd/Ctrl+Enter saves, like the add dialog.
            if (
              isEditable &&
              e.key === "Enter" &&
              (e.metaKey || e.ctrlKey) &&
              !isSaving
            ) {
              e.preventDefault();
              document
                .getElementById("edit-tx-form")
                ?.dispatchEvent(
                  new Event("submit", { bubbles: true, cancelable: true })
                );
            }
          }}
        >
          <DialogHeader className="gap-3">
            <div className="flex items-start gap-3 pr-6 text-left">
              {headerCategory ? (
                <CategoryTile category={headerCategory} size="md" />
              ) : (
                <Tile size="md" className={headerVisual.chip}>
                  <headerVisual.icon />
                </Tile>
              )}
              <div className="min-w-0 flex-1">
                <span className="text-muted-foreground block text-xs">
                  {live ? formatDate(live.date) : ""} · {accountName}
                  {via && (
                    <>
                      {" "}
                      ·{" "}
                      <span className="text-foreground/80">via {via}</span>
                    </>
                  )}
                </span>
                <DialogTitle className="mt-1 line-clamp-2 text-xl leading-tight break-words">
                  {title}
                </DialogTitle>
                {live && (
                  <span
                    className={cn(
                      "font-figures mt-1.5 block text-2xl leading-none sm:hidden",
                      headerVisual.text
                    )}
                  >
                    <TransactionAmount
                      transaction={live}
                      direction={headerVisual.sign}
                    />
                  </span>
                )}
              </div>
              {live && (
                <span
                  className={cn(
                    "font-figures hidden shrink-0 text-2xl leading-none sm:block",
                    headerVisual.text
                  )}
                >
                  <TransactionAmount
                    transaction={live}
                    direction={headerVisual.sign}
                  />
                </span>
              )}
            </div>
            <DialogDescription className="sr-only">
              {isReimbursement
                ? "A repayment received against money owed to you."
                : isOpeningBalance
                  ? "What the account held when you added it. It sets the starting balance and isn't counted as income."
                  : isEditable
                    ? "Update transaction details."
                    : "Transfer and investment transactions cannot be edited. Delete and recreate if needed."}
            </DialogDescription>
            {isOpeningBalance && (
              <p className="text-muted-foreground text-sm">
                What the account held when you added it. It sets the starting
                balance and isn&apos;t counted as income.
              </p>
            )}
          </DialogHeader>

          {error && (
            <div
              role="alert"
              className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
            >
              {error}
            </div>
          )}

          {isEditable ? (
            <>
              <form
                id="edit-tx-form"
                onSubmit={handleSubmit}
                className="grid gap-4 sm:grid-cols-2"
              >
                {!isOpeningBalance && (
                  <div className="sm:col-span-2">
                    <Segmented
                      full
                      aria-label="Transaction type"
                      value={type === "income" ? "income" : "expense"}
                      onValueChange={(v) => handleTypeChange(v)}
                      options={[
                        {
                          value: "expense" as const,
                          label: "Expense",
                          disabled: isSaving,
                        },
                        {
                          value: "income" as const,
                          label: "Income",
                          disabled: isSaving || isShared,
                        },
                      ]}
                    />
                    {isShared && (
                      <p className="text-muted-foreground mt-1.5 text-xs">
                        Remove the money owed back to change the type.
                      </p>
                    )}
                  </div>
                )}

                <div className="flex flex-col gap-1.5 sm:col-span-2">
                  <Label htmlFor="edit-tx-amount">Amount</Label>
                  <CurrencyInput
                    id="edit-tx-amount"
                    value={amount}
                    onChange={setAmount}
                    currency={inputCurrency}
                    placeholder="0.00"
                    disabled={isSaving}
                    className="font-figures h-14 pl-16 text-3xl font-medium md:text-3xl"
                  />
                </div>

                {!isOpeningBalance && (
                  <>
                    <div className="flex flex-col gap-1.5">
                      <Label htmlFor="edit-tx-category">Category</Label>
                      <Select
                        value={categoryId}
                        onValueChange={setCategoryId}
                        disabled={isSaving}
                      >
                        <SelectTrigger id="edit-tx-category" className="w-full">
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
                    <div className="flex flex-col gap-1.5">
                      <Label htmlFor="edit-tx-account">Account</Label>
                      <Select
                        value={accountId}
                        onValueChange={setAccountId}
                        disabled={isSaving}
                      >
                        <SelectTrigger id="edit-tx-account" className="w-full">
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
                  </>
                )}

                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="edit-tx-date">Date</Label>
                  <Input
                    id="edit-tx-date"
                    type="date"
                    value={date}
                    onChange={(e) => setDate(e.target.value)}
                    disabled={isSaving}
                  />
                </div>

                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="edit-tx-description">Description</Label>
                  <Input
                    id="edit-tx-description"
                    placeholder="What was it for?"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    disabled={isSaving}
                    maxLength={500}
                  />
                </div>

                <div className="flex flex-col gap-1.5 sm:col-span-2">
                  <Label htmlFor="edit-tx-notes">Note</Label>
                  <Textarea
                    id="edit-tx-notes"
                    value={notes}
                    onChange={(e) => setNotes(e.target.value)}
                    maxLength={2000}
                    disabled={isSaving}
                    placeholder="Anything worth remembering"
                    className="min-h-[64px]"
                  />
                  {notes.length > 1800 && (
                    <span className="text-muted-foreground self-end text-xs">
                      {notes.length}/2000
                    </span>
                  )}
                </div>

                {/* Receipts */}
                {transaction && (
                  <div className="sm:col-span-2">
                    <ExistingAttachments txId={transaction.id} />
                  </div>
                )}
              </form>

              {/* Money owed back (expenses only). Lives outside the form so
                  Enter in its inputs never submits the main form. */}
              {live?.type === "expense" && (
                <section
                  aria-label="Money owed back"
                  className="flex flex-col gap-3 rounded-card border p-3"
                >
                  {isShared && (
                    <>
                      <div className="flex flex-col gap-0.5 text-sm sm:flex-row sm:items-center sm:justify-between sm:gap-2">
                        <span className="font-medium">Shared expense</span>
                        <span className="text-muted-foreground">
                          Your share{" "}
                          <Money
                            amount={sharedPersonal}
                            currency={txCurrency}
                            className="text-foreground font-medium"
                          />{" "}
                          of{" "}
                          <Money
                            amount={live.amount}
                            currency={txCurrency}
                            className="text-foreground font-medium"
                          />
                          . Budgets count your share only.
                        </span>
                      </div>
                      <ReceivableList
                        receivables={receivables}
                        expense={toReceivableExpense(live)}
                        showExpense={false}
                      />
                    </>
                  )}
                  <ReceivablesEditor key={editorKey} transaction={live} />
                </section>
              )}

              {/* Rule suggestion */}
              {canSuggestRule && (
                <div className="bg-muted/50 flex items-center gap-2.5 rounded-card px-3 py-2.5 text-sm">
                  <Filter
                    className="text-muted-foreground size-4 shrink-0"
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1">
                    Always put &ldquo;{description.trim()}&rdquo; in{" "}
                    {suggestedCategory?.name ?? "this category"}?{" "}
                    <button
                      type="button"
                      onClick={handleCreateRule}
                      disabled={isSaving}
                      className="text-primary font-medium underline-offset-4 hover:underline"
                    >
                      Create rule
                    </button>
                  </span>
                </div>
              )}

              <DialogFooter className="mt-1 flex-row items-center gap-2 sm:justify-between">
                <Button
                  type="button"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => setDeleteOpen(true)}
                  disabled={isSaving || isDeleting}
                >
                  Delete
                </Button>
                <span className="ml-auto flex items-center gap-2">
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => onOpenChange(false)}
                    disabled={isSaving}
                  >
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    form="edit-tx-form"
                    disabled={isSaving || isDeleting}
                  >
                    {isSaving ? "Saving…" : "Save"}
                  </Button>
                </span>
              </DialogFooter>
            </>
          ) : isReimbursement && transaction ? (
            <ReimbursementView
              transaction={transaction}
              accountName={accountName}
              currency={txCurrency}
              onNavigate={() => onOpenChange(false)}
            />
          ) : (
            /* Read-only view for transfer/investment */
            <div className="flex flex-col gap-3">
              <div className="flex items-center gap-2">
                <Label>Type</Label>
                <Badge variant="secondary" className="capitalize">
                  {transaction?.type}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <Label>Amount</Label>
                <span className="text-sm font-medium">
                  {transaction ? (
                    <TransactionAmount transaction={transaction} />
                  ) : (
                    ""
                  )}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <Label>Account</Label>
                <span className="text-sm">
                  {transaction?.account?.name ?? "—"}
                </span>
              </div>
              {transaction?.to_account && (
                <div className="flex items-center justify-between">
                  <Label>To Account</Label>
                  <span className="text-sm">{transaction.to_account.name}</span>
                </div>
              )}
              <div className="flex items-center justify-between">
                <Label>Description</Label>
                <span className="text-sm">
                  {transaction?.description || "—"}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <Label>Date</Label>
                <span className="text-sm">
                  {transaction ? formatDate(transaction.date) : ""}
                </span>
              </div>
              {live?.notes && (
                <div className="flex flex-col gap-1">
                  <Label>Note</Label>
                  <p className="text-sm whitespace-pre-wrap">{live.notes}</p>
                </div>
              )}

              {/* Receipts */}
              {transaction && <ExistingAttachments txId={transaction.id} />}

              <DialogFooter>
                <Button
                  variant="destructive"
                  onClick={() => setDeleteOpen(true)}
                  disabled={isDeleting}
                >
                  Delete Transaction
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Delete confirmation */}
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Transaction</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete this transaction? This will reverse
              its balance impact on the account.
              {isShared &&
                " The money owed back on this expense will be removed too."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDelete}
              disabled={isDeleting}
            >
              {isDeleting ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Create-rule-from-transaction (plan 018, PR 3) */}
      <RuleDialog
        open={ruleOpen}
        onOpenChange={setRuleOpen}
        initialConditions={ruleSeed.conditions}
        initialCategoryId={ruleSeed.categoryId}
      />
    </>
  );
}
