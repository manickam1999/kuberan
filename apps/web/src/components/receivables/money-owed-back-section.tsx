"use client";

import { ChevronDown, ChevronRight, Plus, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatCurrency } from "@/lib/format";
import {
  MAX_COUNTERPARTIES,
  OWED_MODES,
  computeYourShare,
  counterpartyKey,
  nextRowKey,
  resolveShareMode,
  type OwedMode,
} from "@/lib/shared-expense";
import { Button } from "@/components/ui/button";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Label } from "@/components/ui/label";
import { CounterpartyInput } from "./counterparty-input";
import type { ShareMode } from "@/types/models";

export interface OwedRow {
  key: string;
  name: string;
  /** cents; only used in custom mode */
  amount: number;
}

export interface OwedBackState {
  /** Section expanded and in use. */
  enabled: boolean;
  mode: OwedMode;
  splitMode: "equal" | "custom";
  partialClaim: boolean;
  rows: OwedRow[];
}

export function newOwedRow(): OwedRow {
  return { key: nextRowKey(), name: "", amount: 0 };
}

export function initialOwedBackState(): OwedBackState {
  return {
    enabled: false,
    mode: "split",
    splitMode: "equal",
    partialClaim: false,
    rows: [newOwedRow()],
  };
}

/** Wire-level share mode for the current selection. */
export function owedShareMode(state: OwedBackState): ShareMode {
  return resolveShareMode(state.mode, state.splitMode, state.partialClaim);
}

/** Rows after a mode change: full-style modes keep exactly one row. */
function normalizeRows(next: OwedBackState): OwedBackState {
  const shareMode = owedShareMode(next);
  const isFull = shareMode === "full";
  const single = next.mode === "claim" && next.partialClaim;
  let rows = next.rows.length > 0 ? next.rows : [newOwedRow()];
  if (isFull || single) rows = rows.slice(0, 1);
  return { ...next, rows };
}

interface MoneyOwedBackSectionProps {
  state: OwedBackState;
  onChange: (state: OwedBackState) => void;
  /** Expense amount in cents. */
  amount: number;
  disabled?: boolean;
}

/**
 * Collapsible "Money owed back" section for the create-expense form. Pure
 * controlled component; the parent validates and submits.
 */
export function MoneyOwedBackSection({
  state,
  onChange,
  amount,
  disabled,
}: MoneyOwedBackSectionProps) {
  const shareMode = owedShareMode(state);
  const modeInfo = OWED_MODES.find((m) => m.value === state.mode) ?? OWED_MODES[0];
  const singleRow = shareMode === "full" || (state.mode === "claim" && state.partialClaim);
  const yourShare = computeYourShare(
    amount,
    shareMode,
    state.rows.map((r) => r.amount)
  );

  function update(patch: Partial<OwedBackState>) {
    onChange(normalizeRows({ ...state, ...patch }));
  }

  function updateRow(key: string, patch: Partial<OwedRow>) {
    onChange({
      ...state,
      rows: state.rows.map((r) => (r.key === key ? { ...r, ...patch } : r)),
    });
  }

  return (
    <div className="rounded-lg border">
      <button
        type="button"
        className="flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm font-medium"
        aria-expanded={state.enabled}
        aria-controls="owed-back-panel"
        onClick={() => update({ enabled: !state.enabled })}
        disabled={disabled}
      >
        {state.enabled ? (
          <ChevronDown className="size-4" aria-hidden="true" />
        ) : (
          <ChevronRight className="size-4" aria-hidden="true" />
        )}
        Money owed back
        <span className="ml-auto text-xs font-normal text-muted-foreground">
          Optional
        </span>
      </button>

      {state.enabled && (
        <div id="owed-back-panel" className="flex flex-col gap-3 border-t px-3 py-3">
          <div
            role="radiogroup"
            aria-label="Type of money owed back"
            className="flex flex-wrap gap-1.5"
          >
            {OWED_MODES.map((m) => {
              const active = state.mode === m.value;
              return (
                <button
                  key={m.value}
                  type="button"
                  role="radio"
                  aria-checked={active}
                  disabled={disabled}
                  onClick={() => update({ mode: m.value })}
                  className={cn(
                    "rounded-full border px-3 py-1 text-xs font-medium transition-colors disabled:opacity-50",
                    active
                      ? "border-primary bg-primary/10 text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {m.label}
                </button>
              );
            })}
          </div>
          <p className="text-xs text-muted-foreground">{modeInfo.hint}</p>

          {state.mode === "split" && (
            <div
              role="radiogroup"
              aria-label="Split method"
              className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
            >
              {(["equal", "custom"] as const).map((sm) => (
                <button
                  key={sm}
                  type="button"
                  role="radio"
                  aria-checked={state.splitMode === sm}
                  disabled={disabled}
                  onClick={() => update({ splitMode: sm })}
                  className={cn(
                    "rounded-md px-2 py-1.5 text-sm font-medium transition-all disabled:opacity-50",
                    state.splitMode === sm
                      ? "bg-background shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {sm === "equal" ? "Equal" : "Custom"}
                </button>
              ))}
            </div>
          )}

          {state.mode === "claim" && (
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                checked={state.partialClaim}
                disabled={disabled}
                onChange={(e) => update({ partialClaim: e.target.checked })}
              />
              Partially covered
            </label>
          )}

          <div className="flex flex-col gap-2">
            {state.rows.map((row, i) => {
              const others = state.rows
                .filter((r) => r.key !== row.key)
                .map((r) => counterpartyKey(r.name));
              const nameId = `owed-name-${i}`;
              const amountId = `owed-amount-${i}`;
              const label = singleRow
                ? modeInfo.counterpartyLabel
                : `${modeInfo.counterpartyLabel} ${i + 1}`;
              return (
                <div key={row.key} className="flex items-start gap-2">
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <Label htmlFor={nameId} className="text-xs text-muted-foreground">
                      {label}
                    </Label>
                    <CounterpartyInput
                      id={nameId}
                      value={row.name}
                      onChange={(name) => updateRow(row.key, { name })}
                      disabled={disabled}
                      excludeKeys={others}
                    />
                  </div>
                  {shareMode === "custom" && (
                    <div className="flex w-32 shrink-0 flex-col gap-1">
                      <Label htmlFor={amountId} className="text-xs text-muted-foreground">
                        {`Amount owed${singleRow ? "" : ` ${i + 1}`}`}
                      </Label>
                      <CurrencyInput
                        id={amountId}
                        value={row.amount}
                        onChange={(amt) => updateRow(row.key, { amount: amt })}
                        disabled={disabled}
                        placeholder="0.00"
                      />
                    </div>
                  )}
                  {!singleRow && state.rows.length > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="mt-5 shrink-0"
                      aria-label={`Remove ${row.name.trim() || `${modeInfo.counterpartyLabel.toLowerCase()} ${i + 1}`}`}
                      disabled={disabled}
                      onClick={() =>
                        onChange({
                          ...state,
                          rows: state.rows.filter((r) => r.key !== row.key),
                        })
                      }
                    >
                      <X className="size-4" />
                    </Button>
                  )}
                </div>
              );
            })}
            {!singleRow && state.rows.length < MAX_COUNTERPARTIES && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="self-start"
                disabled={disabled}
                onClick={() =>
                  onChange({ ...state, rows: [...state.rows, newOwedRow()] })
                }
              >
                <Plus className="size-4" />
                Add person
              </Button>
            )}
          </div>

          <div
            className="flex items-center justify-between rounded-md bg-muted/50 px-3 py-2 text-sm"
            data-testid="your-share"
          >
            <span className="text-muted-foreground">Your share</span>
            <output
              aria-label="Your share"
              className={cn("money font-semibold", yourShare < 0 && "text-destructive")}
            >
              {formatCurrency(yourShare)}
            </output>
          </div>
          {yourShare === 0 && (
            <p className="-mt-1 text-xs text-muted-foreground">
              Nothing counts as your spending, so a category is optional.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
