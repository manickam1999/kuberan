"use client";

import {
  LayoutDashboard,
  Wallet,
  ArrowLeftRight,
  Tag,
  Filter,
  PiggyBank,
  HandCoins,
  Database,
  TrendingUp,
  Plus,
  Receipt,
  FolderPlus,
  Handshake,
} from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";

export type QuickAction = "account" | "transaction" | "lend" | "category";

interface CommandPaletteMenuProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNavigate: (path: string) => void;
  onAction: (action: QuickAction) => void;
}

const NAV_ITEMS = [
  { path: "/", label: "Dashboard", icon: LayoutDashboard },
  { path: "/accounts", label: "Accounts", icon: Wallet },
  { path: "/transactions", label: "Transactions", icon: ArrowLeftRight },
  { path: "/categories", label: "Categories", icon: Tag },
  { path: "/rules", label: "Rules", icon: Filter },
  { path: "/budgets", label: "Budgets", icon: PiggyBank },
  { path: "/receivables", label: "Owed to me", icon: HandCoins },
  { path: "/securities", label: "Securities", icon: Database },
  { path: "/investments", label: "Investments", icon: TrendingUp },
] as const;

const ACTION_ITEMS = [
  { action: "account", label: "Create Account", icon: Plus },
  { action: "transaction", label: "Add Transaction", icon: Receipt },
  { action: "lend", label: "Lend money", icon: Handshake },
  { action: "category", label: "Create Category", icon: FolderPlus },
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
          {NAV_ITEMS.map(({ path, label, icon: Icon }) => (
            <CommandItem key={path} onSelect={() => onNavigate(path)}>
              <Icon />
              <span>{label}</span>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Quick Actions">
          {ACTION_ITEMS.map(({ action, label, icon: Icon }) => (
            <CommandItem key={action} onSelect={() => onAction(action)}>
              <Icon />
              <span>{label}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
