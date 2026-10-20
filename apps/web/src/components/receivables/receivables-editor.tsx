"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ChevronDown, ChevronRight, Plus, X } from "lucide-react";
import { ApiClientError } from "@/lib/api-client";
import { formatCurrency } from "@/lib/format";
import {
  RECEIVABLE_KINDS,
  RECEIVABLE_KIND_LABELS,
  receivableErrorMessage,
} from "@/lib/receivable-format";
import {
  MAX_COUNTERPARTIES,
  buildSharedCounterparties,
  counterpartyKey,
  equalShareAmount,
  nextRowKey,
  normalizeCounterpartyName,
} from "@/lib/shared-expense";
import { useUpdateReceivables } from "@/hooks/use-receivables";
import { Button } from "@/components/ui/button";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { CounterpartyInput } from "./counterparty-input";
import type { Receivable, ReceivableKind, Transaction } from "@/types/models";

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
    hasExisting ? rowsFrom(existing) : [{ key: nextRowKey(), name: "", amount: 0, paid: 0, writtenOff: 0 }]
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
        onError: (err) => {
          if (err instanceof ApiClientError) {
            setError(receivableErrorMessage(err.code) ?? err.message);
          } else {
            setError("An unexpected error occurred");
          }
        },
      }
    );
  }

  if (!open) {
    return (
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="self-start"
        aria-expanded={false}
        onClick={() => setOpen(true)}
      >
        <ChevronRight className="size-4" aria-hidden="true" />
        {hasExisting ? "Edit people and amounts" : "Money owed back"}
      </Button>
    );
  }

  return (
    <div className="flex flex-col gap-3 rounded-lg border p-3">
      <button
        type="button"
        className="flex items-center gap-2 text-left text-sm font-medium"
        aria-expanded={true}
        onClick={() => setOpen(false)}
      >
        <ChevronDown className="size-4" aria-hidden="true" />
        {hasExisting ? "Edit people and amounts" : "Money owed back"}
      </button>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="recv-kind">Kind</Label>
        <Select
          value={kind}
          onValueChange={(v) => setKind(v as ReceivableKind)}
          disabled={update.isPending}
        >
          <SelectTrigger id="recv-kind" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RECEIVABLE_KINDS.map((k) => (
              <SelectItem key={k} value={k}>
                {RECEIVABLE_KIND_LABELS[k]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <TooltipProvider>
        <div className="flex flex-col gap-2">
          {rows.map((row, i) => {
            const locked = row.paid > 0;
            const others = rows
              .filter((r) => r.key !== row.key)
              .map((r) => counterpartyKey(r.name));
            return (
              <div key={row.key} className="flex items-start gap-2">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <Label
                    htmlFor={`recv-name-${i}`}
                    className="text-xs text-muted-foreground"
                  >
                    {`Person ${i + 1}`}
                  </Label>
                  <CounterpartyInput
                    id={`recv-name-${i}`}
                    value={row.name}
                    onChange={(name) => patchRow(row.key, { name })}
                    disabled={update.isPending}
                    excludeKeys={others}
                  />
                </div>
                <div className="flex w-32 shrink-0 flex-col gap-1">
                  <Label
                    htmlFor={`recv-amount-${i}`}
                    className="text-xs text-muted-foreground"
                  >
                    {`Amount owed ${i + 1}`}
                  </Label>
                  <CurrencyInput
                    id={`recv-amount-${i}`}
                    value={row.amount}
                    onChange={(amount) => patchRow(row.key, { amount })}
                    disabled={update.isPending}
                    placeholder="0.00"
                  />
                </div>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="mt-5 shrink-0" tabIndex={locked ? 0 : -1}>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={`Remove ${row.name.trim() || `person ${i + 1}`}`}
                        disabled={locked || update.isPending}
                        onClick={() =>
                          setRows((rs) => rs.filter((r) => r.key !== row.key))
                        }
                      >
                        <X className="size-4" />
                      </Button>
                    </span>
                  </TooltipTrigger>
                  {locked && (
                    <TooltipContent>
                      Already paid {formatCurrency(row.paid)}. Undo the repayments
                      first to remove.
                    </TooltipContent>
                  )}
                </Tooltip>
              </div>
            );
          })}
        </div>
      </TooltipProvider>

      <div className="flex flex-wrap gap-2">
        {rows.length < MAX_COUNTERPARTIES && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={update.isPending}
            onClick={() =>
              setRows((rs) => [
                ...rs,
                { key: nextRowKey(), name: "", amount: 0, paid: 0, writtenOff: 0 },
              ])
            }
          >
            <Plus className="size-4" />
            Add person
          </Button>
        )}
        {rows.length > 0 && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={update.isPending}
            onClick={splitEqually}
          >
            Split equally
          </Button>
        )}
      </div>

      <div className="flex items-center justify-between rounded-md bg-muted/50 px-3 py-2 text-sm">
        <span className="text-muted-foreground">Your share</span>
        <output
          aria-label="Your share"
          className="money font-semibold"
        >
          {formatCurrency(yourShare)}
        </output>
      </div>
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

      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={update.isPending}
          onClick={() => setOpen(false)}
        >
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={update.isPending}
          onClick={handleSave}
        >
          {update.isPending ? "Saving…" : "Save money owed back"}
        </Button>
      </div>
    </div>
  );
}
