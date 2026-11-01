"use client";

import { useEffect, useState, forwardRef, useImperativeHandle } from "react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import type { QuickAction } from "./command-palette-menu";

// Everything behind the palette is loaded on first use. The create dialogs
// fetch accounts/categories when mounted, so mounting them eagerly in the
// layout cost several requests on every page load.
const CommandPaletteMenu = dynamic(() => import("./command-palette-menu"), {
  ssr: false,
});
const CreateAccountDialog = dynamic(
  () =>
    import("@/components/accounts/create-account-dialog").then(
      (m) => m.CreateAccountDialog
    ),
  { ssr: false }
);
const CreateTransactionDialog = dynamic(
  () =>
    import("@/components/transactions/create-transaction-dialog").then(
      (m) => m.CreateTransactionDialog
    ),
  { ssr: false }
);
const LendMoneyDialog = dynamic(
  () =>
    import("@/components/receivables/lend-money-dialog").then(
      (m) => m.LendMoneyDialog
    ),
  { ssr: false }
);
const CreateCategoryDialog = dynamic(
  () =>
    import("@/components/categories/create-category-dialog").then(
      (m) => m.CreateCategoryDialog
    ),
  { ssr: false }
);

export interface CommandPaletteRef {
  toggle: () => void;
}

export const CommandPalette = forwardRef<CommandPaletteRef>(
  function CommandPalette(_, ref) {
    const router = useRouter();
    const [open, setOpen] = useState(false);
    // Once something has been opened it stays mounted, so closing it can
    // still animate out.
    const [menuUsed, setMenuUsed] = useState(false);
    const [activeAction, setActiveAction] = useState<QuickAction | null>(null);
    const [usedActions, setUsedActions] = useState<ReadonlySet<QuickAction>>(
      () => new Set()
    );

    const setPaletteOpen = (next: boolean | ((prev: boolean) => boolean)) => {
      setMenuUsed(true);
      setOpen(next);
    };

    // Expose toggle method to parent components
    useImperativeHandle(ref, () => ({
      toggle: () => setPaletteOpen((prev) => !prev),
    }));

    // Keyboard listener for Cmd+K / Ctrl+K
    useEffect(() => {
      const down = (e: KeyboardEvent) => {
        if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          setMenuUsed(true);
          setOpen((prev) => !prev);
        }
      };

      document.addEventListener("keydown", down);
      return () => document.removeEventListener("keydown", down);
    }, []);

    const navigateTo = (path: string) => {
      setOpen(false);
      router.push(path);
    };

    const openAction = (action: QuickAction) => {
      setOpen(false);
      setUsedActions((prev) => new Set(prev).add(action));
      setActiveAction(action);
    };

    const dialogProps = (action: QuickAction) => ({
      open: activeAction === action,
      onOpenChange: (next: boolean) => setActiveAction(next ? action : null),
    });

    return (
      <>
        {menuUsed && (
          <CommandPaletteMenu
            open={open}
            onOpenChange={setOpen}
            onNavigate={navigateTo}
            onAction={openAction}
          />
        )}

        {usedActions.has("account") && (
          <CreateAccountDialog {...dialogProps("account")} />
        )}
        {usedActions.has("transaction") && (
          <CreateTransactionDialog {...dialogProps("transaction")} />
        )}
        {usedActions.has("lend") && <LendMoneyDialog {...dialogProps("lend")} />}
        {usedActions.has("category") && (
          <CreateCategoryDialog {...dialogProps("category")} />
        )}
      </>
    );
  }
);
