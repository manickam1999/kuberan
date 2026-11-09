"use client";

import {
  Plus,
  Receipt,
  FolderPlus,
  Handshake,
  Keyboard,
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
import { PALETTE_NAV } from "@/lib/nav";

export type QuickAction = "account" | "transaction" | "lend" | "category";

interface CommandPaletteMenuProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNavigate: (path: string) => void;
  onAction: (action: QuickAction) => void;
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

/**
 * The palette's dialog and menu. Split from CommandPalette so cmdk is only
 * downloaded the first time the palette opens.
 */
export default function CommandPaletteMenu({
  open,
  onOpenChange,
  onNavigate,
  onAction,
}: CommandPaletteMenuProps) {
  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>

        <CommandGroup heading="Navigation">
          {PALETTE_NAV.map(({ href, title, icon: Icon, goKey }) => (
            <CommandItem key={href} onSelect={() => onNavigate(href)}>
              <Icon />
              <span>{title}</span>
              {goKey && <CommandShortcut>G {goKey.toUpperCase()}</CommandShortcut>}
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Quick Actions">
          {ACTION_ITEMS.map(({ action, label, icon: Icon, hint }) => (
            <CommandItem key={action} onSelect={() => onAction(action)}>
              <Icon />
              <span>{label}</span>
              {hint && <CommandShortcut>{hint}</CommandShortcut>}
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Keyboard shortcuts">
          {OTHER_SHORTCUTS.map(({ label, keys }) => (
            <CommandItem
              key={label}
              // Informational rows: selecting one just closes the palette.
              onSelect={() => onOpenChange(false)}
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
