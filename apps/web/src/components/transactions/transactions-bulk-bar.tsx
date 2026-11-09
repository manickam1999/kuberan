"use client";

import { useState } from "react";
import { toast } from "sonner";
import { PencilLine, Tag, Trash2, X } from "lucide-react";

import { ApiClientError } from "@/lib/api-client";
import { sameCurrencySum } from "@/lib/transaction-list";
import { useBulkUpdateTransactions } from "@/hooks/use-transactions";
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
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Money } from "@/components/ui/money";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import { CategoryPicker } from "@/components/transactions/category-picker";
import type { Category, Transaction } from "@/types/models";

/** The API accepts at most this many ids per bulk call. */
export const BULK_LIMIT = 200;

function bulkError(err: unknown): string {
  if (err instanceof ApiClientError) {
    if (err.code === "CATEGORY_TYPE_MISMATCH") {
      return "That category doesn't fit every selected transaction (income and expense categories differ).";
    }
    return err.message;
  }
  return "Couldn't update the selected transactions";
}

interface BulkBarProps {
  selected: Transaction[];
  categories: Category[];
  currencyOf: (tx: Transaction) => string;
  onClear: () => void;
}

/** Actions for the selected rows: recategorise, append a note, delete. */
export function TransactionsBulkBar({
  selected,
  categories,
  currencyOf,
  onClear,
}: BulkBarProps) {
  const bulk = useBulkUpdateTransactions();
  const [noteOpen, setNoteOpen] = useState(false);
  const [note, setNote] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);

  const ids = selected.map((t) => t.id);
  const count = ids.length;
  const tooMany = count > BULK_LIMIT;
  const sum = sameCurrencySum(selected, currencyOf);

  // Income and expense categories can't be mixed in one call.
  const types = new Set(selected.map((t) => t.type));
  const onlyType = types.size === 1 ? Array.from(types)[0] : null;
  const categorisable = onlyType === "expense" || onlyType === "income";
  const options = categorisable
    ? categories.filter((c) => c.type === onlyType)
    : [];

  function run(
    body: Parameters<typeof bulk.mutate>[0],
    done: string,
    after?: () => void
  ) {
    bulk.mutate(body, {
      onSuccess: () => {
        toast.success(done);
        after?.();
        onClear();
      },
      onError: (err) => toast.error(bulkError(err)),
    });
  }

  const plural = count === 1 ? "transaction" : "transactions";

  return (
    <>
      <div
        role="region"
        aria-label="Bulk actions"
        className="bg-elevated fixed inset-x-3 bottom-[calc(var(--tabbar-h)+0.75rem)] z-30 flex items-center gap-2 rounded-card border p-2 pl-3 shadow-lg md:sticky md:inset-x-auto md:top-2 md:bottom-auto md:z-20 md:shadow-md"
      >
        <span className="text-sm font-semibold" aria-live="polite">
          {count} selected
        </span>
        {sum && (
          <Money
            amount={sum.amount}
            currency={sum.currency}
            className="font-figures text-muted-foreground text-sm"
          />
        )}
        {tooMany && (
          <span className="text-destructive text-xs">
            Select at most {BULK_LIMIT} at a time.
          </span>
        )}
        <span className="min-w-0 flex-1" />

        <CategoryPicker
          categories={options}
          value={null}
          // The bulk API can't clear a category, so don't offer "No category".
          allowNone={false}
          onSelect={(id) =>
            id &&
            run(
              { ids, set: { category_id: id } },
              `Moved ${count} ${plural} to ${
                options.find((c) => c.id === id)?.name ?? "the category"
              }`
            )
          }
          align="end"
        >
          <Button
            variant="outline"
            size="sm"
            disabled={!categorisable || tooMany || bulk.isPending}
            title={
              categorisable
                ? undefined
                : "Select only expenses, or only income, to change the category"
            }
          >
            <Tag />
            Category
          </Button>
        </CategoryPicker>

        <Popover open={noteOpen} onOpenChange={setNoteOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              disabled={tooMany || bulk.isPending}
              aria-label="Add note"
            >
              <PencilLine />
              <span className="max-sm:sr-only">Add note</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-72">
            <form
              className="grid gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                const text = note.trim();
                if (!text) return;
                run(
                  { ids, set: { notes_append: text } },
                  `Added a note to ${count} ${plural}`,
                  () => {
                    setNote("");
                    setNoteOpen(false);
                  }
                );
              }}
            >
              <div className="grid gap-1.5">
                <Label htmlFor="bulk-note">Note to add</Label>
                <Textarea
                  id="bulk-note"
                  value={note}
                  maxLength={2000}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Added on a new line after each note"
                  className="min-h-[72px]"
                />
              </div>
              <Button type="submit" size="sm" disabled={!note.trim() || bulk.isPending}>
                Add to {count}
              </Button>
            </form>
          </PopoverContent>
        </Popover>

        <Button
          variant="outline"
          size="sm"
          className="text-destructive hover:text-destructive"
          onClick={() => setConfirmOpen(true)}
          disabled={tooMany || bulk.isPending}
          aria-label="Delete selected"
        >
          <Trash2 />
          <span className="max-sm:sr-only">Delete</span>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClear}
          aria-label="Clear selection"
        >
          <X />
        </Button>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {count} {plural}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This reverses their effect on your account balances. Money owed
              back on any shared expense among them is removed too. This can&apos;t
              be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={bulk.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={bulk.isPending}
              onClick={(e) => {
                e.preventDefault();
                run({ ids, delete: true }, `Deleted ${count} ${plural}`, () =>
                  setConfirmOpen(false)
                );
              }}
            >
              {bulk.isPending ? "Deleting…" : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
