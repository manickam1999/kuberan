"use client";

import { ChevronDown, ChevronRight } from "lucide-react";
import {
  OWED_MODES,
  computeYourShare,
  equalShareAmount,
  nextRowKey,
  resolveShareMode,
  type OwedMode,
} from "@/lib/shared-expense";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ChoiceChips } from "./choice-chips";
import { CounterpartyRows, YourShare } from "./counterparty-rows";
import type { ShareMode } from "@/types/models";

const SPLIT_METHODS = [
  { value: "equal", label: "Equal" },
  { value: "custom", label: "Custom" },
] as const;

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

/** True when only one person can be listed (full-style modes). */
function isSingleRow(state: OwedBackState): boolean {
  return owedShareMode(state) === "full" || (state.mode === "claim" && state.partialClaim);
}

/**
 * Rows that count for the current selection. Extra rows are kept in state when
 * the user switches to a one-person mode and only hidden, so switching back
 * restores them. Always use this (not `state.rows`) for totals and submission.
 */
export function visibleOwedRows(state: OwedBackState): OwedRow[] {
  const rows = state.rows.length > 0 ? state.rows : [newOwedRow()];
  return isSingleRow(state) ? rows.slice(0, 1) : rows;
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
  const singleRow = isSingleRow(state);
  const rows = visibleOwedRows(state);
  const yourShare = computeYourShare(
    amount,
    shareMode,
    rows.map((r) => r.amount)
  );

  function update(patch: Partial<OwedBackState>) {
    onChange({ ...state, ...patch });
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
          <ChoiceChips
            label="Type of money owed back"
            value={state.mode}
            onValueChange={(mode) => update({ mode })}
            options={OWED_MODES}
            disabled={disabled}
          />
          <p className="text-xs text-muted-foreground">{modeInfo.hint}</p>

          {state.mode === "split" && (
            <ChoiceChips
              label="Split method"
              variant="segmented"
              value={state.splitMode}
              onValueChange={(splitMode) => update({ splitMode })}
              options={SPLIT_METHODS}
              disabled={disabled}
            />
          )}

          {state.mode === "claim" && (
            <div className="flex items-center gap-2 text-sm">
              <Checkbox
                id="owed-partial-claim"
                checked={state.partialClaim}
                disabled={disabled}
                onCheckedChange={(v) => update({ partialClaim: v === true })}
              />
              <Label htmlFor="owed-partial-claim" className="font-normal">
                Partially covered
              </Label>
            </div>
          )}

          <CounterpartyRows
            rows={rows}
            idPrefix="owed"
            noun={modeInfo.counterpartyLabel}
            numbered={!singleRow}
            showAmounts={shareMode === "custom"}
            equalShare={
              shareMode === "equal" ? equalShareAmount(amount, rows.length) : undefined
            }
            disabled={disabled}
            onNameChange={(key, name) => updateRow(key, { name })}
            onAmountChange={(key, amt) => updateRow(key, { amount: amt })}
            onRemove={
              !singleRow && rows.length > 1
                ? (key) =>
                    onChange({ ...state, rows: state.rows.filter((r) => r.key !== key) })
                : undefined
            }
            onAdd={
              singleRow
                ? undefined
                : () => onChange({ ...state, rows: [...rows, newOwedRow()] })
            }
          />

          <YourShare cents={yourShare} />
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
