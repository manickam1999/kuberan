"use client";

import { useEffect, useState } from "react";
import { useCounterparties } from "@/hooks/use-receivables";
import { Input } from "@/components/ui/input";
import type { Counterparty } from "@/types/api";

export interface CounterpartyInputProps {
  value: string;
  onChange: (name: string) => void;
  id?: string;
  placeholder?: string;
  disabled?: boolean;
  /** Counterparty keys to hide from the suggestions (e.g. the one being renamed). */
  excludeKeys?: string[];
  /** Called when a suggestion is picked. */
  onPick?: (counterparty: Counterparty) => void;
}

/**
 * Text input with autocomplete from GET /receivables/counterparties. The typed
 * text is the source of truth; suggestions are a convenience. Suggestions show
 * inline beneath the input while it has focus.
 */
export function CounterpartyInput({
  value,
  onChange,
  id,
  placeholder = "Name",
  disabled,
  excludeKeys = [],
  onPick,
}: CounterpartyInputProps) {
  const [focused, setFocused] = useState(false);
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), 200);
    return () => clearTimeout(t);
  }, [value]);

  const { data } = useCounterparties(debounced.trim(), focused);
  const suggestions = (data ?? []).filter(
    (c) =>
      !excludeKeys.includes(c.key) &&
      c.name.toLowerCase() !== value.trim().toLowerCase()
  );

  return (
    <div className="space-y-1">
      <Input
        id={id}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        maxLength={100}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        // Delay so a click on a suggestion registers before it unmounts.
        onBlur={() => setTimeout(() => setFocused(false), 150)}
      />
      {focused && suggestions.length > 0 && (
        <ul className="max-h-40 overflow-auto rounded-md border bg-popover p-1 text-sm shadow-sm">
          {suggestions.map((c) => (
            <li key={c.key}>
              <button
                type="button"
                className="w-full rounded-sm px-2 py-1.5 text-left hover:bg-accent"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onChange(c.name);
                  onPick?.(c);
                }}
              >
                {c.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
