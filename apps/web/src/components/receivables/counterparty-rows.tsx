"use client";

import type { ReactNode } from "react";
import { Plus, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatCurrency } from "@/lib/format";
import { MAX_COUNTERPARTIES, counterpartyKey } from "@/lib/shared-expense";
import { Button } from "@/components/ui/button";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { CounterpartyInput } from "./counterparty-input";

export interface CounterpartyRowData {
  key: string;
  name: string;
  /** cents */
  amount: number;
  /** When set the row can't be removed; shown as the tooltip. */
  lockedReason?: string;
}

interface CounterpartyRowsProps {
  rows: CounterpartyRowData[];
  /** Prefix for the input ids, e.g. "owed" gives owed-name-0 / owed-amount-0. */
  idPrefix: string;
  /** Singular label for who owes, e.g. "Person" or "Held by". */
  noun: string;
  /** Append " 1", " 2"... to labels (off when there is only ever one row). */
  numbered: boolean;
  /** Show an editable amount next to each name. */
  showAmounts: boolean;
  /** Equal split: read-only amount each person owes, shown under the name. */
  equalShare?: number;
  onNameChange: (key: string, name: string) => void;
  onAmountChange: (key: string, amount: number) => void;
  /** Omit to hide the remove buttons. */
  onRemove?: (key: string) => void;
  /** Omit to hide the "Add person" button. */
  onAdd?: () => void;
  disabled?: boolean;
  /** Extra buttons shown beside "Add person". */
  actions?: ReactNode;
}

/**
 * Presentational list of counterparty name/amount rows shared by the create
 * form and the edit-time receivables editor. Stacks name over amount on phones
 * so the name field never gets squeezed.
 */
export function CounterpartyRows({
  rows,
  idPrefix,
  noun,
  numbered,
  showAmounts,
  equalShare,
  onNameChange,
  onAmountChange,
  onRemove,
  onAdd,
  disabled,
  actions,
}: CounterpartyRowsProps) {
  return (
    <TooltipProvider>
      <div className="flex flex-col gap-2">
        {rows.map((row, i) => {
          const others = rows
            .filter((r) => r.key !== row.key)
            .map((r) => counterpartyKey(r.name));
          const suffix = numbered ? ` ${i + 1}` : "";
          const nameId = `${idPrefix}-name-${i}`;
          const amountId = `${idPrefix}-amount-${i}`;
          const locked = Boolean(row.lockedReason);
          return (
            <div key={row.key} className="flex items-start gap-2">
              <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <Label htmlFor={nameId} className="text-xs text-muted-foreground">
                    {`${noun}${suffix}`}
                  </Label>
                  <CounterpartyInput
                    id={nameId}
                    value={row.name}
                    onChange={(name) => onNameChange(row.key, name)}
                    disabled={disabled}
                    excludeKeys={others}
                  />
                  {equalShare !== undefined && equalShare > 0 && (
                    <p className="money text-xs text-muted-foreground">
                      owes {formatCurrency(equalShare)}
                    </p>
                  )}
                </div>
                {showAmounts && (
                  <div className="flex shrink-0 flex-col gap-1 sm:w-32">
                    <Label
                      htmlFor={amountId}
                      className="text-xs text-muted-foreground"
                    >
                      {`Amount owed${suffix}`}
                    </Label>
                    <CurrencyInput
                      id={amountId}
                      value={row.amount}
                      onChange={(amt) => onAmountChange(row.key, amt)}
                      disabled={disabled}
                      placeholder="0.00"
                    />
                  </div>
                )}
              </div>
              {onRemove && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="mt-5 shrink-0" tabIndex={locked ? 0 : -1}>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={`Remove ${row.name.trim() || `${noun.toLowerCase()}${suffix}`}`}
                        disabled={locked || disabled}
                        onClick={() => onRemove(row.key)}
                      >
                        <X className="size-4" />
                      </Button>
                    </span>
                  </TooltipTrigger>
                  {locked && <TooltipContent>{row.lockedReason}</TooltipContent>}
                </Tooltip>
              )}
            </div>
          );
        })}
        {(onAdd || actions) && (
          <div className="flex flex-wrap gap-2">
            {onAdd && rows.length < MAX_COUNTERPARTIES && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={disabled}
                onClick={onAdd}
              >
                <Plus className="size-4" />
                Add person
              </Button>
            )}
            {actions}
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}

/** "Your share" summary row. Keeps the accessible name "Your share". */
export function YourShare({
  cents,
  className,
}: {
  cents: number;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center justify-between rounded-md bg-muted/50 px-3 py-2 text-sm",
        className
      )}
      data-testid="your-share"
    >
      <span className="text-muted-foreground">Your share</span>
      <output
        aria-label="Your share"
        className={cn("money font-semibold", cents < 0 && "text-destructive")}
      >
        {formatCurrency(cents)}
      </output>
    </div>
  );
}
