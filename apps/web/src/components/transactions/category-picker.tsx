"use client";

import { useMemo, useState, type ReactNode } from "react";
import { Check } from "lucide-react";

import { cn } from "@/lib/utils";
import { CategoryIcon } from "@/components/categories/category-icon";
import { Tile } from "@/components/ui/tile";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { Category } from "@/types/models";

/** Parents and their children as one flat, de-duplicated list. */
export function flattenCategories(categories: Category[]): Category[] {
  const seen = new Set<string>();
  const out: Category[] = [];
  const add = (c: Category) => {
    if (seen.has(c.id)) return;
    seen.add(c.id);
    out.push(c);
  };
  for (const c of categories) {
    add(c);
    for (const child of c.children ?? []) add(child);
  }
  return out;
}

/** A category's colour tile (icon on a wash of its colour). */
export function CategoryTile({
  category,
  size = "sm",
}: {
  category: Pick<Category, "icon" | "name" | "color">;
  size?: "sm" | "md";
}) {
  return (
    <Tile size={size} color={category.color || undefined} tone="neutral">
      <CategoryIcon icon={category.icon} name={category.name} />
    </Tile>
  );
}

interface CategoryPickerProps {
  categories: Category[];
  /** Selected category id; empty or null for none. */
  value?: string | null;
  onSelect: (categoryId: string | null) => void;
  /** Offer a "No category" row. */
  allowNone?: boolean;
  /** Label of that row (a filter says "Any category"). */
  noneLabel?: string;
  /** The trigger element (rendered `asChild`). */
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  align?: "start" | "center" | "end";
  className?: string;
}

/** Searchable category list in a popover, opened from any trigger. */
export function CategoryPicker({
  categories,
  value,
  onSelect,
  allowNone = true,
  noneLabel = "No category",
  children,
  open,
  onOpenChange,
  align = "start",
  className,
}: CategoryPickerProps) {
  const [innerOpen, setInnerOpen] = useState(false);
  const isOpen = open ?? innerOpen;
  const setOpen = (next: boolean) => {
    setInnerOpen(next);
    onOpenChange?.(next);
  };
  const flat = useMemo(
    () =>
      flattenCategories(categories).sort((a, b) => a.name.localeCompare(b.name)),
    [categories]
  );

  return (
    <Popover open={isOpen} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent align={align} className={cn("w-64 p-0", className)}>
        <Command>
          <CommandInput placeholder="Find a category" />
          <CommandList>
            <CommandEmpty>No categories found.</CommandEmpty>
            {allowNone && (
              <CommandItem
                value="__none__ no category uncategorised"
                onSelect={() => {
                  onSelect(null);
                  setOpen(false);
                }}
              >
                <span className="text-muted-foreground">{noneLabel}</span>
                {!value && <Check className="ml-auto size-4" />}
              </CommandItem>
            )}
            {flat.map((c) => (
              <CommandItem
                key={c.id}
                value={`${c.name} ${c.id}`}
                onSelect={() => {
                  onSelect(c.id);
                  setOpen(false);
                }}
              >
                <CategoryTile category={c} />
                <span className="truncate">{c.name}</span>
                {value === c.id && <Check className="ml-auto size-4" />}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
