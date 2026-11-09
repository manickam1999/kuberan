"use client";

import * as React from "react";
import { Divide, Equal, Minus, Plus, X } from "lucide-react";
import { currencySymbol } from "@/lib/currencies";
import {
  AMOUNT_INPUT_PATTERN,
  evaluateAmount,
  evaluateAmountLenient,
  formatCentsPlain,
} from "@/lib/money-expr";
import { cn } from "@/lib/utils";

interface CurrencyInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type"> {
  /** Value in cents */
  value: number;
  /** Called with value in cents */
  onChange: (cents: number) => void;
  /** ISO 4217 code of the amount; shown as its en-MY symbol (RM, US$, ...). */
  currency: string;
  /**
   * Allow arithmetic such as `12.50+8*2` (evaluated to cents, with a live
   * preview and operator chips on touch devices). When false the field only
   * accepts a plain number with up to two decimals. Defaults to true.
   */
  allowExpressions?: boolean;
}

const LEGACY_PATTERN = /^\d*\.?\d{0,2}$/;
const OPERATOR_PATTERN = /[+\-*/×÷xX()−]/;
const CHIPS: {
  label: string;
  name: string;
  insert: string;
  Icon: React.ComponentType<{ className?: string }>;
}[] = [
  { label: "+", name: "Plus", insert: "+", Icon: Plus },
  { label: "−", name: "Minus", insert: "-", Icon: Minus },
  { label: "×", name: "Times", insert: "*", Icon: X },
  { label: "÷", name: "Divide", insert: "/", Icon: Divide },
];

const previewFormatter = new Intl.NumberFormat("en-MY", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

function formatInitial(cents: number): string {
  return formatCentsPlain(cents);
}

function CurrencyInput({
  value,
  onChange,
  currency,
  className,
  allowExpressions = true,
  onBlur,
  onKeyDown,
  onFocus,
  "aria-invalid": ariaInvalid,
  ...props
}: CurrencyInputProps) {
  const [displayValue, setDisplayValue] = React.useState(() =>
    formatInitial(value)
  );
  const [invalidReason, setInvalidReason] = React.useState<string | null>(null);
  const [focused, setFocused] = React.useState(false);
  const [coarse, setCoarse] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const lastEmitted = React.useRef(value);

  React.useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    setCoarse(window.matchMedia("(pointer: coarse)").matches);
  }, []);

  // Sync display when the parent sets a value we didn't emit (reset, FX
  // prefill, ...). Values we emitted ourselves are skipped so an expression
  // being typed is never overwritten by its own result.
  React.useEffect(() => {
    if (value === lastEmitted.current) return;
    lastEmitted.current = value;
    setDisplayValue(formatCentsPlain(value));
    setInvalidReason(null);
  }, [value]);

  function emit(cents: number) {
    lastEmitted.current = cents;
    onChange(cents);
  }

  /** Shared path for typing and chip insertion. Returns false if rejected. */
  function applyText(raw: string): boolean {
    if (raw === "") {
      setDisplayValue("");
      setInvalidReason(null);
      emit(0);
      return true;
    }

    if (!allowExpressions) {
      if (!LEGACY_PATTERN.test(raw)) return false;
      setDisplayValue(raw);
      setInvalidReason(null);
      emit(raw === "." ? 0 : Math.round(parseFloat(raw) * 100));
      return true;
    }

    if (!AMOUNT_INPUT_PATTERN.test(raw)) return false;
    if (!OPERATOR_PATTERN.test(raw) && /\.\d{3,}/.test(raw)) return false;

    setDisplayValue(raw);
    setInvalidReason(null);
    const result = evaluateAmount(raw);
    if (result.ok) emit(result.cents);
    return true;
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    applyText(e.target.value);
  }

  function commit() {
    if (displayValue === "") {
      setDisplayValue("0.00");
      return;
    }
    if (!allowExpressions) {
      setDisplayValue(formatCentsPlain(Math.round(parseFloat(displayValue) * 100) || 0));
      return;
    }
    const result = evaluateAmountLenient(displayValue);
    if (result.ok) {
      setDisplayValue(formatCentsPlain(result.cents));
      setInvalidReason(null);
      emit(result.cents);
    } else if (result.reason === "empty") {
      setDisplayValue("0.00");
    } else {
      // Don't leave a stale value in the form while the field shows an error.
      emit(0);
      setInvalidReason(
        result.reason === "div_zero"
          ? "Can't divide by zero"
          : "Can't calculate this"
      );
    }
  }

  function handleBlur(e: React.FocusEvent<HTMLInputElement>) {
    commit();
    setFocused(false);
    onBlur?.(e);
  }

  function handleFocus(e: React.FocusEvent<HTMLInputElement>) {
    setFocused(true);
    onFocus?.(e);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (allowExpressions) {
      if (e.key === "=") {
        e.preventDefault();
        commit();
      } else if (e.key === "Enter" && displayValue !== "") {
        const r = evaluateAmount(displayValue);
        if (!(r.ok && !r.isExpression)) {
          e.preventDefault();
          commit();
        }
      }
    }
    onKeyDown?.(e);
  }

  function insertOperator(insert: string) {
    const el = inputRef.current;
    const start = el?.selectionStart ?? displayValue.length;
    const end = el?.selectionEnd ?? displayValue.length;
    const next = displayValue.slice(0, start) + insert + displayValue.slice(end);
    if (!applyText(next)) return;
    const caret = start + insert.length;
    requestAnimationFrame(() => {
      inputRef.current?.setSelectionRange(caret, caret);
    });
  }

  const evaluation =
    allowExpressions && displayValue !== "" ? evaluateAmount(displayValue) : null;
  const previewCents =
    evaluation && evaluation.ok && evaluation.isExpression
      ? evaluation.cents
      : null;
  const showPreview = previewCents !== null;
  const showChips = allowExpressions && coarse && focused;

  return (
    <div>
      <div className="relative">
        <span className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm">
          {currencySymbol(currency)}
        </span>
        <input
          ref={inputRef}
          type="text"
          inputMode="decimal"
          data-slot="input"
          className={cn(
            "file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input h-10 w-full min-w-0 rounded-md border bg-transparent py-1 pr-3 pl-12 text-base shadow-xs transition-[color,box-shadow] outline-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
            "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
            "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
            className,
            showPreview && "pr-28"
          )}
          value={displayValue}
          onChange={handleChange}
          onBlur={handleBlur}
          onFocus={handleFocus}
          onKeyDown={handleKeyDown}
          aria-invalid={ariaInvalid || invalidReason !== null ? true : undefined}
          {...props}
        />
        <span
          aria-live="polite"
          className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2"
        >
          {showPreview && (
            <span className="money bg-primary/10 text-primary animate-in fade-in-0 zoom-in-95 block rounded-md px-2 py-0.5 text-sm font-semibold tabular-nums duration-150">
              <span aria-hidden="true">
                = {previewFormatter.format(previewCents / 100)}
              </span>
              <span className="sr-only">
                equals {currencySymbol(currency)}{" "}
                {previewFormatter.format(previewCents / 100)}
              </span>
            </span>
          )}
        </span>
      </div>
      {invalidReason && (
        <p role="alert" className="text-destructive mt-1.5 text-xs">
          {invalidReason}
        </p>
      )}
      {showChips && (
        <div className="bg-muted animate-in fade-in-0 slide-in-from-top-1 mt-2 grid grid-cols-5 gap-1 rounded-xl p-1 duration-150">
          {CHIPS.map(({ label, name, insert, Icon }) => (
            <button
              key={label}
              type="button"
              tabIndex={-1}
              aria-label={name}
              onPointerDown={(e) => e.preventDefault()}
              onClick={() => insertOperator(insert)}
              className="text-muted-foreground active:bg-background active:text-foreground flex h-10 items-center justify-center rounded-lg transition-colors"
            >
              <Icon className="size-4" />
            </button>
          ))}
          <button
            type="button"
            tabIndex={-1}
            aria-label="Equals"
            onPointerDown={(e) => e.preventDefault()}
            onClick={commit}
            className="bg-primary/15 text-primary active:bg-background flex h-10 items-center justify-center rounded-lg transition-colors"
          >
            <Equal className="size-4" />
          </button>
        </div>
      )}
    </div>
  );
}

export { CurrencyInput };
export type { CurrencyInputProps };
