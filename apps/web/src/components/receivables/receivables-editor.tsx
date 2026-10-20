"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ChevronDown, ChevronRight } from "lucide-react";
import { formatCurrency } from "@/lib/format";
import {
  RECEIVABLE_KINDS,
  RECEIVABLE_KIND_LABELS,
  receivableApiErrorMessage,
} from "@/lib/receivable-format";
import {
  buildSharedCounterparties,
  equalShareAmount,
  nextRowKey,
  normalizeCounterpartyName,
} from "@/lib/shared-expense";
import { useUpdateReceivables } from "@/hooks/use-receivables";
import { Button } from "@/components/ui/button";
import { ChoiceChips } from "./choice-chips";
import { CounterpartyRows, YourShare } from "./counterparty-rows";
import type { Receivable, ReceivableKind, Transaction } from "@/types/models";

const KIND_OPTIONS = RECEIVABLE_KINDS.map((k) => ({
  value: k,
  label: RECEIVABLE_KIND_LABELS[k],
}));

function newRow(): EditorRow {
  return { key: nextRowKey(), name: "", amount: 0, paid: 0, writtenOff: 0 };
}

interface EditorRow {
  key: string;
  /** Present for existing receivables; absent for new rows. */
  id?: string;
  name: string;
  amount: number;
  paid: number;
  writtenOff: number;
}

function rowsFrom(receivables: Receivable[]): EditorRow[] {
  return receivables.map((r) => ({
    key: r.id,
    id: r.id,
    name: r.counterparty_name,
    amount: r.amount,
    paid: r.paid_amount,
    writtenOff: r.written_off_amount,
  }));
}

export interface ReceivablesEditorProps {
  /** The saved expense. Remount (via `key`) when its receivables change. */
  transaction: Transaction;
  /** Called after a successful save. */
  onSaved?: () => void;
}

/**
 * Inline editor for an expense's receivables. Builds the diff payload for
 * PUT /transactions/:id/receivables: existing rows keep their `id`, new rows
 * have none, and rows left out are removed (disabled once repayments exist).
 * Also used to mark an expense that has none as shared.
 */
export function ReceivablesEditor({ transaction, onSaved }: ReceivablesEditorProps) {
  const existing = transaction.receivables ?? [];
  const hasExisting = existing.length > 0;
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<EditorRow[]>(() =>
    hasExisting ? rowsFrom(existing) : [newRow()]
  );
  const [kind, setKind] = useState<ReceivableKind>(existing[0]?.kind ?? "split");
  const [error, setError] = useState("");
  const update = useUpdateReceivables(transaction.id);

  const total = transaction.amount;
  const yourShare =
    total - rows.reduce((s, r) => s + (r.amount - r.writtenOff), 0);

  function patchRow(key: string, patch: Partial<EditorRow>) {
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  }

  function splitEqually() {
    const each = equalShareAmount(total, rows.length);
    setRows((rs) =>
      rs.map((r) => ({ ...r, amount: Math.max(each, r.paid + r.writtenOff) }))
    );
  }

  function handleSave() {
    setError("");
    if (rows.length === 0 && !hasExisting) {
      setError("Add at least one person");
      return;
    }

    let receivables: { id?: string; name: string; amount: number }[] = [];
    if (rows.length > 0) {
      const built = buildSharedCounterparties({
        amount: total,
        mode: "custom",
        rows,
      });
      if (!built.ok) {
        setError(built.error);
        return;
      }
      const tooLow = rows.find((r) => r.amount < r.paid + r.writtenOff);
      if (tooLow) {
        setError(
          `${normalizeCounterpartyName(tooLow.name)} has already paid or been written off ${formatCurrency(tooLow.paid + tooLow.writtenOff)}, so the amount can't be lower than that.`
        );
        return;
      }
      receivables = rows.map((r) => ({
        ...(r.id ? { id: r.id } : {}),
        name: normalizeCounterpartyName(r.name),
        amount: r.amount,
      }));
    }

    update.mutate(
      {
        // Kind applies to every receivable on the expense; only send it when
        // it is new or changed.
        ...(!hasExisting || existing.some((r) => r.kind !== kind) ? { kind } : {}),
        receivables,
      },
      {
        onSuccess: () => {
          toast.success(
            receivables.length === 0
              ? "Money owed back removed"
              : hasExisting
                ? "Money owed back updated"
                : "Expense marked as shared"
          );
          setOpen(false);
          onSaved?.();
        },
        onError: (err) => setError(receivableApiErrorMessage(err)),
      }
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {/* One toggle stays mounted so keyboard focus is never dropped. */}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="self-start"
        aria-expanded={open}
        aria-controls="recv-editor-panel"
        onClick={() => setOpen((o) => !o)}
      >
        {open ? (
          <ChevronDown className="size-4" aria-hidden="true" />
        ) : (
          <ChevronRight className="size-4" aria-hidden="true" />
        )}
        {hasExisting ? "Edit people and amounts" : "Money owed back"}
      </Button>

      {open && (
        <div
          id="recv-editor-panel"
          className="flex flex-col gap-3 rounded-lg border p-3"
        >
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Kind</span>
            <ChoiceChips
              label="Kind"
              value={kind}
              onValueChange={setKind}
              options={KIND_OPTIONS}
              disabled={update.isPending}
            />
          </div>

          <CounterpartyRows
            rows={rows.map((r) => ({
              key: r.key,
              name: r.name,
              amount: r.amount,
              lockedReason:
                r.paid > 0
                  ? `Already paid ${formatCurrency(r.paid)}. Undo the repayments first to remove.`
                  : undefined,
            }))}
            idPrefix="recv"
            noun="Person"
            numbered
            showAmounts
            disabled={update.isPending}
            onNameChange={(key, name) => patchRow(key, { name })}
            onAmountChange={(key, amount) => patchRow(key, { amount })}
            onRemove={(key) => setRows((rs) => rs.filter((r) => r.key !== key))}
            onAdd={() => setRows((rs) => [...rs, newRow()])}
            actions={
              rows.length > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={update.isPending}
                  onClick={splitEqually}
                >
                  Split equally
                </Button>
              )
            }
          />

          <YourShare cents={yourShare} />
          {rows.length === 0 && hasExisting && (
            <p className="text-xs text-muted-foreground">
              Nobody will owe you for this expense, so all of it counts as your
              spending.
            </p>
          )}

          {error && (
            <div
              role="alert"
              className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
            >
              {error}
            </div>
          )}

          {/* Secondary on purpose: this saves straight to the server and is
              separate from the dialog's primary "Save changes". */}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={update.isPending}
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={update.isPending}
              onClick={handleSave}
            >
              {update.isPending ? "Saving…" : "Save people & amounts"}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
