"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";
import { CATEGORY_ICONS, isSingleEmoji, namedCategoryIcon } from "@/lib/category-icons";
import { Input } from "@/components/ui/input";

interface CategoryIconPickerProps {
  id?: string;
  value: string;
  onChange: (icon: string) => void;
  disabled?: boolean;
}

/**
 * Picks a category icon: one of the named icons, or a single emoji typed into
 * the field below the grid. Clicking the selected icon clears it.
 */
export function CategoryIconPicker({ id, value, onChange, disabled }: CategoryIconPickerProps) {
  const isNamed = namedCategoryIcon(value) !== undefined;
  // Text typed that isn't a valid emoji yet. While it's set the field shows it
  // and `value` keeps the last valid icon; otherwise the field mirrors `value`.
  const [draft, setDraft] = useState<string | null>(null);
  const emoji = draft ?? (isNamed ? "" : value);
  const emojiInvalid = emoji !== "" && !isSingleEmoji(emoji);

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-8 gap-1.5" role="radiogroup" aria-label="Icon">
        {CATEGORY_ICONS.map(({ key, icon: Icon }) => {
          const selected = value === key;
          return (
            <button
              key={key}
              type="button"
              role="radio"
              aria-checked={selected}
              aria-label={key.replace(/-\d+$/, "").replace(/-/g, " ")}
              title={key}
              disabled={disabled}
              onClick={() => {
                setDraft(null);
                onChange(selected ? "" : key);
              }}
              className={cn(
                "flex aspect-square items-center justify-center rounded-md border text-muted-foreground transition-colors",
                "hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                selected && "border-primary bg-primary/10 text-primary",
                disabled && "cursor-not-allowed opacity-50"
              )}
            >
              <Icon className="size-4" />
            </button>
          );
        })}
      </div>
      <div className="flex flex-col gap-1">
        <Input
          id={id}
          placeholder="Or type one emoji, e.g. 🛒"
          value={emoji}
          onChange={(e) => {
            const next = e.target.value.trim();
            if (next === "" || isSingleEmoji(next)) {
              setDraft(null);
              onChange(next);
            } else {
              setDraft(next);
            }
          }}
          disabled={disabled}
          aria-invalid={emojiInvalid}
          maxLength={16}
        />
        {emojiInvalid && (
          <p className="text-xs text-destructive">Use a single emoji, or pick an icon above.</p>
        )}
      </div>
    </div>
  );
}
