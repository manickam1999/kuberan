"use client";

import { RadioGroup } from "radix-ui";
import { cn } from "@/lib/utils";

export interface ChoiceOption<T extends string> {
  value: T;
  label: string;
}

interface ChoiceChipsProps<T extends string> {
  value: T;
  onValueChange: (value: T) => void;
  options: readonly ChoiceOption<T>[];
  /** Accessible name of the group. */
  label: string;
  /** "chips" wraps rounded pills; "segmented" is an equal-width muted track. */
  variant?: "chips" | "segmented";
  disabled?: boolean;
  className?: string;
}

/**
 * Single-select choice built on a Radix radio group: one Tab stop, arrow keys
 * move and select, and every target is at least 40px tall on touch screens.
 */
export function ChoiceChips<T extends string>({
  value,
  onValueChange,
  options,
  label,
  variant = "chips",
  disabled,
  className,
}: ChoiceChipsProps<T>) {
  const segmented = variant === "segmented";
  return (
    <RadioGroup.Root
      value={value}
      onValueChange={(v) => onValueChange(v as T)}
      aria-label={label}
      disabled={disabled}
      orientation="horizontal"
      className={cn(
        segmented
          ? "grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
          : "flex flex-wrap gap-1.5",
        className
      )}
    >
      {options.map((o) => (
        <RadioGroup.Item
          key={o.value}
          value={o.value}
          className={cn(
            "inline-flex min-h-10 items-center justify-center text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50 sm:min-h-8",
            segmented
              ? "rounded-md px-2 py-1.5 text-muted-foreground hover:text-foreground data-[state=checked]:bg-background data-[state=checked]:text-foreground data-[state=checked]:shadow-sm"
              : "rounded-full border px-3 py-1 text-muted-foreground hover:text-foreground data-[state=checked]:border-primary data-[state=checked]:bg-primary/10 data-[state=checked]:text-foreground"
          )}
        >
          {o.label}
        </RadioGroup.Item>
      ))}
    </RadioGroup.Root>
  );
}
