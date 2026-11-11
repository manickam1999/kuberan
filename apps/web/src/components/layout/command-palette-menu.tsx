"use client";

import { useState } from "react";
import { useTheme } from "next-themes";
import {
  Plus,
  Receipt,
  FolderPlus,
  Handshake,
  Keyboard,
  Settings,
  SunMoon,
} from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import { Money } from "@/components/ui/money";
import { CategoryTile } from "@/components/dashboard/home-bits";
import { useAccountCurrencies } from "@/hooks/use-accounts";
import {
  SEARCH_MIN_CHARS,
  useDebouncedValue,
  useTransactionSearch,
} from "@/hooks/use-transaction-search";
import { transactionVisual } from "@/lib/domain-visuals";
import { formatDate } from "@/lib/format";
import { PALETTE_NAV } from "@/lib/nav";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/types/models";

export type QuickAction = "account" | "transaction" | "lend" | "category";

interface CommandPaletteMenuProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNavigate: (path: string) => void;
  onAction: (action: QuickAction) => void;
  /** A transaction result was chosen. */
  onTransaction: (transaction: Transaction) => void;
}

const ACTION_ITEMS = [
  { action: "account", label: "Create Account", icon: Plus, hint: "" },
  { action: "transaction", label: "Add Transaction", icon: Receipt, hint: "N" },
  { action: "lend", label: "Lend money", icon: Handshake, hint: "" },
  { action: "category", label: "Create Category", icon: FolderPlus, hint: "" },
] as const;

/** Global shortcuts that aren't tied to a palette entry. */
const OTHER_SHORTCUTS = [
  { label: "Open this palette", keys: "⌘K  or  /" },
  { label: "Toggle sidebar", keys: "⌘B" },
] as const;

/** Wraps the part of `text` that matches `query` in bold. */
function Highlight({ text, query }: { text: string; query: string }) {
  const q = query.trim();
  const i = q ? text.toLowerCase().indexOf(q.toLowerCase()) : -1;
  if (i < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, i)}
      <b className="font-semibold">{text.slice(i, i + q.length)}</b>
      {text.slice(i + q.length)}
    </>
  );
}

/**
 * The palette's dialog and menu. Split from CommandPalette so cmdk is only
 * downloaded the first time the palette opens.
 */
export default function CommandPaletteMenu({
  open,
  onOpenChange,
  onNavigate,
  onAction,
  onTransaction,
}: CommandPaletteMenuProps) {
  const [query, setQuery] = useState("");
  const debounced = useDebouncedValue(query, 250);
  const search = useTransactionSearch(debounced);
  const currencyOf = useAccountCurrencies();
  const { resolvedTheme, setTheme } = useTheme();

  const searching = query.trim().length >= SEARCH_MIN_CHARS;
  const results = searching ? (search.data?.data ?? []) : [];
  // Results are for the debounced text; show a hint while the next ones load.
  const pending =
    searching && (debounced.trim() !== query.trim() || search.isFetching);

  // The parent closes the palette directly after an action or a jump, so
  // clear the text whenever it goes from open to closed.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (!open) setQuery("");
  }

  const handleOpenChange = (next: boolean) => onOpenChange(next);

  return (
    <CommandDialog open={open} onOpenChange={handleOpenChange}>
      <CommandInput
        value={query}
        onValueChange={setQuery}
        placeholder="Search or jump to..."
      />
      {/* Phones: the list fills the sheet and scrolls; desktop keeps a cap. */}
      <CommandList className="max-h-none flex-1 overscroll-contain pb-2 sm:max-h-[min(420px,60vh)] sm:flex-none">
        <CommandEmpty>
          {searching && pending ? "Searching..." : "No results found."}
        </CommandEmpty>

        {searching && (results.length > 0 || pending) && (
          <>
            <CommandGroup heading="Transactions">
              {results.length === 0 && (
                <CommandItem
                  disabled
                  value={`${query} searching`}
                  className="text-muted-foreground"
                >
                  Searching...
                </CommandItem>
              )}
              {results.map((tx) => {
                const v = transactionVisual(tx.type);
                const signed = v.sign === "-" ? -tx.amount : tx.amount;
                const title = tx.description || v.label;
                return (
                  // The server already matched on description, notes and
                  // amount, so the value carries the query to keep cmdk's own
                  // filter from hiding a row it can't match by text.
                  <CommandItem
                    key={tx.id}
                    value={`${query} ${title} ${tx.id}`}
                    onSelect={() => onTransaction(tx)}
                  >
                    <CategoryTile category={tx.category} type={tx.type} />
                    <span className="min-w-0 flex-1 truncate">
                      <Highlight text={title} query={query} />
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {formatDate(tx.date)}
                    </span>
                    <span
                      className={cn(
                        "min-w-20 shrink-0 text-right text-sm font-medium",
                        v.text
                      )}
                    >
                      <Money
                        amount={signed}
                        currency={currencyOf(tx.account_id, tx.account)}
                        sign={
                          v.sign === "+"
                            ? "always"
                            : v.sign === "-"
                              ? "auto"
                              : "never"
                        }
                      />
                    </span>
                  </CommandItem>
                );
              })}
            </CommandGroup>
            <CommandSeparator />
          </>
        )}

        <CommandGroup heading="Actions">
          {ACTION_ITEMS.map(({ action, label, icon: Icon, hint }) => (
            <CommandItem key={action} onSelect={() => onAction(action)}>
              <Icon />
              <span>{label}</span>
              {hint && <CommandShortcut>{hint}</CommandShortcut>}
            </CommandItem>
          ))}
          <CommandItem
            keywords={["dark", "light", "appearance", "mode"]}
            onSelect={() => {
              setTheme(resolvedTheme === "dark" ? "light" : "dark");
              handleOpenChange(false);
            }}
          >
            <SunMoon />
            <span>Switch theme</span>
          </CommandItem>
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Go to">
          {PALETTE_NAV.map(({ href, title, icon: Icon, goKey }) => (
            <CommandItem key={href} onSelect={() => onNavigate(href)}>
              <Icon />
              <span>{title}</span>
              {goKey && <CommandShortcut>G {goKey.toUpperCase()}</CommandShortcut>}
            </CommandItem>
          ))}
          <CommandItem
            keywords={["profile", "preferences", "appearance", "telegram"]}
            onSelect={() => onNavigate("/settings")}
          >
            <Settings />
            <span>Settings</span>
          </CommandItem>
        </CommandGroup>

        <CommandSeparator className="pointer-coarse:hidden" />

        {/* Touch screens have no keyboard to use these with. */}
        <CommandGroup
          heading="Keyboard shortcuts"
          className="pointer-coarse:hidden"
        >
          {OTHER_SHORTCUTS.map(({ label, keys }) => (
            <CommandItem
              key={label}
              // Informational rows: selecting one just closes the palette.
              onSelect={() => handleOpenChange(false)}
              keywords={["shortcut", "keyboard", "keys"]}
            >
              <Keyboard />
              <span>{label}</span>
              <CommandShortcut>{keys}</CommandShortcut>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
