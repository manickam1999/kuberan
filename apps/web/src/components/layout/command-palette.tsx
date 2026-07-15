"use client";

import { useEffect, useState, forwardRef, useImperativeHandle } from "react";
import { useRouter } from "next/navigation";
import {
  LayoutDashboard,
  Wallet,
  ArrowLeftRight,
  Tag,
  PiggyBank,
  Database,
  TrendingUp,
  Plus,
  Receipt,
  FolderPlus,
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
import { CreateAccountDialog } from "@/components/accounts/create-account-dialog";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { CreateCategoryDialog } from "@/components/categories/create-category-dialog";

export interface CommandPaletteRef {
  toggle: () => void;
}

export const CommandPalette = forwardRef<CommandPaletteRef>(
  function CommandPalette(_, ref) {
    const router = useRouter();
    const [open, setOpen] = useState(false);

    // Expose toggle method to parent components
    useImperativeHandle(ref, () => ({
      toggle: () => setOpen((prev) => !prev),
    }));

  // Dialog states for quick actions
  const [createAccountOpen, setCreateAccountOpen] = useState(false);
  const [createTransactionOpen, setCreateTransactionOpen] = useState(false);
  const [createCategoryOpen, setCreateCategoryOpen] = useState(false);

  // Keyboard listener for Cmd+K / Ctrl+K
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
      }
    };

    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  // Navigation handler
  const navigateTo = (path: string) => {
    setOpen(false);
    router.push(path);
  };

  // Action handler
  const openDialog = (dialogSetter: (open: boolean) => void) => {
    setOpen(false);
    dialogSetter(true);
  };

  return (
    <>
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandInput placeholder="Type a command or search..." />
        <CommandList>
          <CommandEmpty>No results found.</CommandEmpty>

          <CommandGroup heading="Navigation">
            <CommandItem onSelect={() => navigateTo("/")}>
              <LayoutDashboard />
              <span>Dashboard</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/accounts")}>
              <Wallet />
              <span>Accounts</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/transactions")}>
              <ArrowLeftRight />
              <span>Transactions</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/categories")}>
              <Tag />
              <span>Categories</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/budgets")}>
              <PiggyBank />
              <span>Budgets</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/securities")}>
              <Database />
              <span>Securities</span>
            </CommandItem>
            <CommandItem onSelect={() => navigateTo("/investments")}>
              <TrendingUp />
              <span>Investments</span>
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Quick Actions">
            <CommandItem onSelect={() => openDialog(setCreateAccountOpen)}>
              <Plus />
              <span>Create Account</span>
            </CommandItem>
            <CommandItem onSelect={() => openDialog(setCreateTransactionOpen)}>
              <Receipt />
              <span>Add Transaction</span>
            </CommandItem>
            <CommandItem onSelect={() => openDialog(setCreateCategoryOpen)}>
              <FolderPlus />
              <span>Create Category</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>

      {/* All action dialogs */}
      <CreateAccountDialog
        open={createAccountOpen}
        onOpenChange={setCreateAccountOpen}
      />
      <CreateTransactionDialog
        open={createTransactionOpen}
        onOpenChange={setCreateTransactionOpen}
      />
      <CreateCategoryDialog
        open={createCategoryOpen}
        onOpenChange={setCreateCategoryOpen}
      />
    </>
  );
});
