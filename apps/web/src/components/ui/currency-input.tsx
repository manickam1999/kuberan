"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

interface CurrencyInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type"> {
  /** Value in cents */
  value: number;
  /** Called with value in cents */
  onChange: (cents: number) => void;
  /** Currency symbol to display (default: "MYR") */
  symbol?: string;
}

function CurrencyInput({
  value,
  onChange,
  symbol = "MYR",
  className,
  ...props
}: CurrencyInputProps) {
  const [displayValue, setDisplayValue] = React.useState(() =>
    formatCentsToDisplay(value)
  );

  // Sync display when value prop changes externally
  React.useEffect(() => {
    const currentCents = parsDisplayToCents(displayValue);
    if (currentCents !== value) {
      setDisplayValue(formatCentsToDisplay(value));
    }
    // Only sync when value prop changes, not on displayValue changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const raw = e.target.value;

    // Allow empty input
    if (raw === "") {
      setDisplayValue("");
      onChange(0);
      return;
    }

    // Only allow digits and a single decimal point, max 2 decimal places
    if (!/^\d*\.?\d{0,2}$/.test(raw)) {
      return;
    }

    setDisplayValue(raw);
    onChange(parsDisplayToCents(raw));
  }

  function handleBlur() {
    // Format nicely on blur
    setDisplayValue(formatCentsToDisplay(parsDisplayToCents(displayValue)));
  }

  return (
    <div className="relative">
      <span className="text-muted-foreground pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm">
        {symbol}
      </span>
      <input
        type="text"
        inputMode="decimal"
        data-slot="input"
        className={cn(
          "file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input h-10 w-full min-w-0 rounded-md border bg-transparent py-1 pr-3 pl-12 text-base shadow-xs transition-[color,box-shadow] outline-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
          "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
          "aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
          className
        )}
        value={displayValue}
        onChange={handleChange}
        onBlur={handleBlur}
        {...props}
      />
    </div>
  );
}

function formatCentsToDisplay(cents: number): string {
  if (cents === 0) return "0.00";
  return (cents / 100).toFixed(2);
}

function parsDisplayToCents(display: string): number {
  if (!display || display === ".") return 0;
  const dollars = parseFloat(display);
  if (isNaN(dollars)) return 0;
  return Math.round(dollars * 100);
}

export { CurrencyInput };
export type { CurrencyInputProps };
