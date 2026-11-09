"use client";

import { useState } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { PencilLine, Receipt, Split, Star } from "lucide-react";
import { ApiClientError, apiClient, getFxAmountRequired, type FxAmountRequired } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import { useAccounts } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency, useSetDefaultAccount } from "@/hooks/use-profile";
import { useCreateSharedExpense } from "@/hooks/use-receivables";
import {
  MoneyOwedBackSection,
  initialOwedBackState,
  owedShareMode,
  visibleOwedRows,
  type OwedBackState,
} from "@/components/receivables/money-owed-back-section";
import { buildSharedCounterparties } from "@/lib/shared-expense";
import {
  useCreateTransaction,
  useCreateTransfer,
  useTransactions,
  transactionKeys,
} from "@/hooks/use-transactions";
import { recentCategoryIds } from "@/lib/transaction-list";
import { CategoryTile } from "@/components/transactions/category-picker";
import { Segmented } from "@/components/ui/segmented";
import { Textarea } from "@/components/ui/textarea";
import { useCategories } from "@/hooks/use-categories";
import {
  CrossCurrencyAmount,
  useCrossAmount,
} from "@/components/transactions/cross-currency-amount";
import { StagedAttachments } from "@/components/transactions/transaction-attachments";
import { attachmentKeys } from "@/hooks/use-attachments";
import type { AttachmentResponse } from "@/types/api";
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
import {
  canBeDefaultAccount,
  resolveDefaultAccountId,
} from "@/lib/default-account";
import { receivableErrorMessage } from "@/lib/receivable-format";
import type { TransactionType } from "@/types/models";

type DialogType = TransactionType | "transfer";

const TYPE_OPTIONS: { value: DialogType; label: string }[] = [
  { value: "expense", label: "Expense" },
  { value: "income", label: "Income" },
  { value: "transfer", label: "Transfer" },
];

/** Notes are capped at 2,000 characters by the API. */
const NOTES_MAX = 2000;
const CHIP_COUNT = 5;

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "SAME_ACCOUNT") {
      return "Cannot transfer to the same account";
    }
    const friendly = receivableErrorMessage(error.code);
    if (friendly) return friendly;
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
  /**
   * Pin the dialog to an account when opening from the account detail page.
   * Without it, the user's saved default account is preselected.
   */
  defaultAccountId?: string;
}

export function CreateTransactionDialog({
  open,
  onOpenChange,
  defaultAccountId,
}: CreateTransactionDialogProps) {
  // Explicit picks; empty means "use the preselected account" (see below).
  const [accountId, setAccountId] = useState<string>("");
  const [type, setType] = useState<DialogType>("expense");
  const [amount, setAmount] = useState(0);
  const [categoryId, setCategoryId] = useState<string>("");
  const [description, setDescription] = useState("");
  const [date, setDate] = useState(todayISO());
  const [error, setError] = useState("");
  const [stagedFiles, setStagedFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);
  const [owed, setOwed] = useState<OwedBackState>(initialOwedBackState);
  const [notes, setNotes] = useState("");
  // The optional extras stay folded away until asked for.
  const [showNotes, setShowNotes] = useState(false);
  const [showReceipts, setShowReceipts] = useState(false);
  const [showSplit, setShowSplit] = useState(false);
  const [showAllCategories, setShowAllCategories] = useState(false);

  // Transfer-specific state
  const [fromAccountId, setFromAccountId] = useState<string>("");
  const [toAccountId, setToAccountId] = useState<string>("");
  // Set when the API asked for the received amount although the cached
  // accounts looked like same-currency (stale data).
  const [fxRequired, setFxRequired] = useState<FxAmountRequired | null>(null);

  const queryClient = useQueryClient();
  const createTransaction = useCreateTransaction();
  const createTransfer = useCreateTransfer();
  const createShared = useCreateSharedExpense();
  const { defaultAccountId: savedDefaultAccountId } = useAuth();
  const setDefaultAccount = useSetDefaultAccount();
  const baseCurrency = useBaseCurrency();
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
  // Quick category chips: the latest ones used for this type, topped up
  // alphabetically so a new user still sees something to tap.
  const { data: recentData } = useTransactions(
    { page_size: 50, type: type === "income" ? "income" : "expense" },
    { enabled: open && type !== "transfer" }
  );
  const chipCategories = (() => {
    const byId = new Map(categories.map((c) => [c.id, c]));
    const ids = recentCategoryIds(
      recentData?.data ?? [],
      type === "income" ? "income" : "expense",
      CHIP_COUNT
    ).filter((id) => byId.has(id));
    for (const c of categories) {
      if (ids.length >= CHIP_COUNT) break;
      if (!ids.includes(c.id)) ids.push(c.id);
    }
    if (
      categoryId &&
      categoryId !== "none" &&
      byId.has(categoryId) &&
      !ids.includes(categoryId)
    ) {
      ids.unshift(categoryId);
    }
    return ids.map((id) => byId.get(id)!);
  })();
  // The account detail page pins its own account; everywhere else fall back to
  // the user's saved default so the common case needs no account selection.
  const preselectedAccountId =
    defaultAccountId ?? resolveDefaultAccountId(savedDefaultAccountId, accounts);
  const selectedAccountId = accountId || preselectedAccountId;
  const selectedFromAccountId = fromAccountId || preselectedAccountId;
  const selectedAccount = accounts.find((a) => a.id === selectedAccountId);
  const isTransfer = type === "transfer";
  const fromAccount = accounts.find((a) => a.id === selectedFromAccountId);
  const toAccount = accounts.find((a) => a.id === toAccountId);
  // The currency the entered amount is in: the source account for a transfer,
  // else the chosen account.
  const amountCurrency = isTransfer
    ? (fromAccount?.currency ?? baseCurrency)
    : (selectedAccount?.currency ?? baseCurrency);
  const receivedCurrency = fxRequired?.currency || toAccount?.currency || "";
  const received = useCrossAmount(
    isTransfer ? amountCurrency : "",
    isTransfer ? receivedCurrency : "",
    amount
  );
  const sharedActive = type === "expense" && owed.enabled;
  const isSubmitting =
    createTransaction.isPending ||
    createTransfer.isPending ||
    createShared.isPending ||
    uploading;

  // For transfer: filter to accounts, exclude selected from-account for to-account
  const toAccounts = accounts.filter(
    (a) => a.is_active && a.id !== selectedFromAccountId
  );

  function resetForm() {
    setAccountId("");
    setType("expense");
    setAmount(0);
    setCategoryId("");
    setDescription("");
    setDate(todayISO());
    setError("");
    setFromAccountId("");
    setToAccountId("");
    setFxRequired(null);
    setStagedFiles([]);
    setOwed(initialOwedBackState());
    setNotes("");
    setShowNotes(false);
    setShowReceipts(false);
    setShowSplit(false);
    setShowAllCategories(false);
  }

  // After "save and add another": keep the type, account(s) and date, clear
  // what belongs to the entry just saved, and go back to the amount.
  function resetForAnother() {
    setAmount(0);
    setCategoryId("");
    setDescription("");
    setNotes("");
    setError("");
    setToAccountId("");
    setFxRequired(null);
    setStagedFiles([]);
    setOwed(initialOwedBackState());
    requestAnimationFrame(() => document.getElementById("tx-amount")?.focus());
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
      setFromAccountId(selectedAccountId);
    } else if (type === "transfer") {
      setAccountId(selectedFromAccountId);
    }
  }

  function handleFromAccountChange(value: string) {
    setFromAccountId(value);
    if (toAccountId === value) {
      setToAccountId("");
    }
  }

  function handleMakeDefault() {
    if (!selectedAccount) return;
    const name = selectedAccount.name;
    setDefaultAccount.mutate(selectedAccount.id, {
      onSuccess: () => toast.success(`${name} is now your default`),
      onError: (err) => toast.error(getErrorMessage(err)),
    });
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    void submit(false);
  }

  /** Saves the entry; with `addAnother` the dialog stays open for the next one. */
  async function submit(addAnother: boolean) {
    if (isSubmitting) return;
    setError("");
    const finish = () => {
      if (addAnother) resetForAnother();
      else handleOpenChange(false);
    };
    const noteText = notes.trim() || undefined;

    if (amount <= 0) {
      setError("Amount must be greater than zero");
      return;
    }

    if (isTransfer) {
      if (!selectedFromAccountId) {
        setError("Please select a source account");
        return;
      }
      if (!toAccountId) {
        setError("Please select a destination account");
        return;
      }
      if (selectedFromAccountId === toAccountId) {
        setError("Cannot transfer to the same account");
        return;
      }

      if (received.needed && received.value <= 0) {
        setError(`Enter the amount received in ${receivedCurrency}`);
        return;
      }

      createTransfer.mutate(
        {
          from_account_id: selectedFromAccountId,
          to_account_id: toAccountId,
          amount,
          to_amount: received.needed ? received.value : undefined,
          description: description.trim() || undefined,
          date: date ? toRFC3339(date) : undefined,
          notes: noteText,
        },
        {
          onSuccess: () => {
            toast.success("Transfer completed");
            finish();
          },
          onError: (err) => {
            const need = getFxAmountRequired(err);
            if (need && need.field === "to_amount") {
              // Reveal the field (even if the accounts looked same-currency)
              // and offer the server's suggestion.
              setFxRequired(need);
              if (need.suggestedAmount) received.setValue(need.suggestedAmount);
              setError(
                `The accounts are in different currencies. Confirm the amount received in ${need.currency}.`
              );
              return;
            }
            setError(getErrorMessage(err));
          },
        }
      );
    } else {
      if (!selectedAccountId) {
        setError("Please select an account");
        return;
      }

      let created;
      try {
        if (sharedActive) {
          const shareMode = owedShareMode(owed);
          const built = buildSharedCounterparties({
            amount,
            currency: amountCurrency,
            mode: shareMode,
            rows: visibleOwedRows(owed),
          });
          if (!built.ok) {
            setError(built.error);
            return;
          }
          created = await createShared.mutateAsync({
            account_id: selectedAccountId,
            category_id:
              categoryId && categoryId !== "none" ? categoryId : undefined,
            amount,
            description: description.trim() || undefined,
            date: date ? toRFC3339(date) : undefined,
            notes: noteText,
            kind: owed.mode,
            mode: shareMode,
            counterparties: built.counterparties,
          });
        } else {
          created = await createTransaction.mutateAsync({
            account_id: selectedAccountId,
            type: type as TransactionType,
            amount,
            category_id:
              categoryId && categoryId !== "none" ? categoryId : undefined,
            description: description.trim() || undefined,
            date: date ? toRFC3339(date) : undefined,
            notes: noteText,
          });
        }
      } catch (err) {
        setError(getErrorMessage(err));
        return;
      }

      const total = stagedFiles.length;
      const failures = await uploadStagedReceipts(created.id);
      if (failures > 0) {
        // The transaction is already saved; per plan R4 the receipts are
        // recoverable from the edit dialog, so give one clear, actionable
        // message rather than silently succeeding.
        toast.error(
          `Transaction saved, but ${failures} of ${total} receipt${
            total === 1 ? "" : "s"
          } failed to upload. Open the transaction to add ${
            failures === 1 ? "it" : "them"
          } again.`
        );
      } else {
        toast.success(sharedActive ? "Shared expense created" : "Transaction created");
      }
      finish();
    }
  }

  // Two-step create: attachments can only be uploaded once the transaction
  // exists, so we upload the staged files sequentially after the create
  // mutation resolves. Individual failures don't block the rest — the
  // transaction itself is already saved. Returns the number that failed.
  async function uploadStagedReceipts(txId: string): Promise<number> {
    if (stagedFiles.length === 0) return 0;
    setUploading(true);
    let failures = 0;
    for (const file of stagedFiles) {
      const form = new FormData();
      form.append("file", file);
      try {
        await apiClient.upload<AttachmentResponse>(
          `/api/v1/transactions/${txId}/attachments`,
          form
        );
      } catch {
        failures += 1;
      }
    }
    setUploading(false);
    // Refresh every cache the new receipts touch: the transaction's own
    // attachment list and detail (so the edit dialog opens fresh) plus the list
    // rows that carry the paperclip indicator.
    queryClient.invalidateQueries({ queryKey: attachmentKeys.list(txId) });
    queryClient.invalidateQueries({ queryKey: transactionKeys.detail(txId) });
    queryClient.invalidateQueries({ queryKey: transactionKeys.lists() });
    queryClient.invalidateQueries({ queryKey: transactionKeys.userLists() });
    return failures;
  }

  const saveLabel = isSubmitting
    ? isTransfer
      ? "Transferring…"
      : "Saving…"
    : isTransfer
      ? "Transfer funds"
      : type === "income"
        ? "Save income"
        : "Save expense";

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        mobileFull
        className="sm:max-w-md"
        onOpenAutoFocus={(e) => {
          // Amount first: land the cursor there, not on the close button.
          e.preventDefault();
          document.getElementById("tx-amount")?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {isTransfer ? "Transfer funds" : "New transaction"}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {isTransfer
              ? "Move money between your accounts."
              : "Record an income or expense."}
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={handleSubmit}
          onKeyDown={(e) => {
            // Cmd/Ctrl+Enter saves; adding Shift saves and starts another.
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void submit(e.shiftKey);
            }
          }}
          className="flex flex-1 flex-col gap-4"
        >
          <Segmented
            full
            aria-label="Transaction type"
            value={type}
            onValueChange={handleTypeChange}
            options={TYPE_OPTIONS.map((o) => ({
              ...o,
              disabled: isSubmitting,
            }))}
          />

          {/* Amount: first and largest */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="tx-amount" className="sr-only">
              Amount
            </Label>
            <CurrencyInput
              id="tx-amount"
              value={amount}
              onChange={setAmount}
              currency={amountCurrency}
              placeholder="0.00"
              disabled={isSubmitting}
              className="font-figures h-16 pl-16 text-4xl font-medium tracking-tight md:text-4xl"
            />
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
            >
              {error}
            </div>
          )}

          {/* Recent categories (expense/income) */}
          {!isTransfer && (
            <div className="flex flex-col gap-2">
              <span className="text-muted-foreground text-xs font-medium">
                Category
              </span>
              <div className="-mx-5 flex gap-2 overflow-x-auto px-5 pb-1 [scrollbar-width:none] sm:mx-0 sm:flex-wrap sm:px-0 [&::-webkit-scrollbar]:hidden">
                {chipCategories.map((c) => {
                  const on = categoryId === c.id;
                  return (
                    <button
                      key={c.id}
                      type="button"
                      aria-pressed={on}
                      disabled={isSubmitting}
                      onClick={() => setCategoryId(on ? "" : c.id)}
                      className={cn(
                        "focus-visible:ring-ring/50 inline-flex h-10 shrink-0 items-center gap-1.5 rounded-control border pr-3 pl-1.5 text-[13px] whitespace-nowrap transition-colors outline-none focus-visible:ring-[3px] disabled:opacity-50 md:h-9",
                        on
                          ? "border-primary bg-primary/10 text-foreground"
                          : "bg-elevated text-muted-foreground hover:text-foreground"
                      )}
                    >
                      <CategoryTile category={c} />
                      {c.name}
                    </button>
                  );
                })}
                <button
                  type="button"
                  aria-expanded={showAllCategories}
                  onClick={() => setShowAllCategories((v) => !v)}
                  className="focus-visible:ring-ring/50 text-muted-foreground hover:text-foreground inline-flex h-10 shrink-0 items-center rounded-control border border-dashed px-3 text-[13px] outline-none focus-visible:ring-[3px] md:h-9"
                >
                  All…
                </button>
              </div>
              {showAllCategories && (
                <Select
                  value={categoryId}
                  onValueChange={setCategoryId}
                  disabled={isSubmitting}
                >
                  <SelectTrigger
                    id="tx-category"
                    aria-label="Category"
                    className="w-full"
                  >
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
              )}
            </div>
          )}

          {/* From / To (transfer) */}
          {isTransfer && (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="tx-from-account">From</Label>
                <Select
                  value={selectedFromAccountId}
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
                          {a.name} ({a.currency})
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="tx-to-account">To</Label>
                <Select
                  value={toAccountId}
                  onValueChange={(v) => {
                    setToAccountId(v);
                    setFxRequired(null);
                  }}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="tx-to-account" className="w-full">
                    <SelectValue placeholder="Destination" />
                  </SelectTrigger>
                  <SelectContent>
                    {toAccounts.map((a) => (
                      <SelectItem key={a.id} value={a.id}>
                        {a.name} ({a.currency})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          {/* Amount received (cross-currency transfer) */}
          {isTransfer && received.needed && (
            <CrossCurrencyAmount
              id="tx-to-amount"
              label={`Amount received (${receivedCurrency})`}
              fromCurrency={amountCurrency}
              fromAmount={amount}
              toCurrency={receivedCurrency}
              value={received.value}
              onChange={received.setValue}
              rate={received.rate}
              disabled={isSubmitting}
            />
          )}

          {/* Description, account, date */}
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="flex flex-col gap-1.5 sm:col-span-2">
              <Label htmlFor="tx-description">Description</Label>
              <Input
                id="tx-description"
                placeholder={isTransfer ? "Optional" : "What was it for?"}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={isSubmitting}
                maxLength={500}
              />
            </div>

            {!isTransfer && !defaultAccountId && (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="tx-account">Account</Label>
                <Select
                  value={selectedAccountId}
                  onValueChange={setAccountId}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="tx-account" className="w-full">
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
                {selectedAccount && canBeDefaultAccount(selectedAccount) && (
                  <div className="flex justify-end">
                    {selectedAccount.id === savedDefaultAccountId ? (
                      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                        <Star className="size-3 fill-current" />
                        Default account
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={handleMakeDefault}
                        disabled={isSubmitting || setDefaultAccount.isPending}
                        className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
                      >
                        <Star className="size-3" />
                        {setDefaultAccount.isPending ? "Saving…" : "Make default"}
                      </button>
                    )}
                  </div>
                )}
              </div>
            )}

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
          </div>

          {/* Optional extras, folded away until asked for */}
          <div className="flex flex-wrap gap-x-4 gap-y-1">
            {type === "expense" && (
              <ExtraToggle
                icon={<Split />}
                label="Split with someone"
                active={showSplit || owed.enabled}
                onClick={() => {
                  const next = !(showSplit || owed.enabled);
                  setShowSplit(next);
                  setOwed({ ...owed, enabled: next });
                }}
              />
            )}
            <ExtraToggle
              icon={<Receipt />}
              label={
                stagedFiles.length > 0
                  ? `Receipts (${stagedFiles.length})`
                  : "Attach receipt"
              }
              active={showReceipts || stagedFiles.length > 0}
              onClick={() => setShowReceipts((v) => !v)}
              hidden={isTransfer}
            />
            <ExtraToggle
              icon={<PencilLine />}
              label="Add note"
              active={showNotes || notes !== ""}
              onClick={() => setShowNotes((v) => !v)}
            />
          </div>

          {(showNotes || notes !== "") && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="tx-notes">Note</Label>
              <Textarea
                id="tx-notes"
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                maxLength={NOTES_MAX}
                disabled={isSubmitting}
                placeholder="Anything worth remembering"
                className="min-h-[72px]"
              />
              {notes.length > NOTES_MAX - 200 && (
                <span className="text-muted-foreground self-end text-xs">
                  {notes.length}/{NOTES_MAX}
                </span>
              )}
            </div>
          )}

          {type === "expense" && (showSplit || owed.enabled) && (
            <MoneyOwedBackSection
              state={owed}
              onChange={(next) => {
                setOwed(next);
                if (!next.enabled) setShowSplit(false);
              }}
              amount={amount}
              currency={amountCurrency}
              disabled={isSubmitting}
            />
          )}

          {!isTransfer && (showReceipts || stagedFiles.length > 0) && (
            <StagedAttachments
              files={stagedFiles}
              onChange={setStagedFiles}
              disabled={isSubmitting}
            />
          )}

          <DialogFooter className="mt-auto items-center sm:justify-between">
            <span className="text-muted-foreground hidden text-xs sm:inline">
              <kbd className="font-sans">⌘</kbd>
              <kbd className="font-sans">↵</kbd> save ·{" "}
              <kbd className="font-sans">⇧</kbd>
              <kbd className="font-sans">⌘</kbd>
              <kbd className="font-sans">↵</kbd> save and add another
            </span>
            <Button type="submit" className="max-sm:w-full" disabled={isSubmitting}>
              {saveLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** A text button that folds an optional section in or out. */
function ExtraToggle({
  icon,
  label,
  active,
  onClick,
  hidden,
}: {
  icon: React.ReactNode;
  label: string;
  active: boolean;
  onClick: () => void;
  hidden?: boolean;
}) {
  if (hidden) return null;
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "focus-visible:ring-ring/50 inline-flex min-h-9 items-center gap-1.5 rounded-sm text-[13px] transition-colors outline-none focus-visible:ring-[3px] [&_svg]:size-4",
        active ? "text-foreground" : "text-muted-foreground hover:text-foreground"
      )}
    >
      {icon}
      {label}
    </button>
  );
}
