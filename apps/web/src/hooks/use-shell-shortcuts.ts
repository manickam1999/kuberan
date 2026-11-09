import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { MAIN_NAV } from "@/lib/nav";

/** How long "G" waits for the second key of a go-to chord. */
const CHORD_MS = 1500;

const TYPING_SELECTOR =
  'input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="textbox"], [role="combobox"]';

/** An open dialog, sheet, menu, popover or listbox owns the keyboard. */
const OVERLAY_SELECTOR =
  '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"], [role="listbox"][data-state="open"]';

/** True when a bare-key shortcut must not fire for this event. */
export function shouldIgnoreShortcut(e: KeyboardEvent): boolean {
  if (e.defaultPrevented || e.isComposing || e.repeat) return true;
  if (e.metaKey || e.ctrlKey || e.altKey) return true;
  const target = e.target;
  if (target instanceof Element && target.closest(TYPING_SELECTOR)) return true;
  return document.querySelector(OVERLAY_SELECTOR) !== null;
}

interface ShellShortcuts {
  /** `N`: new transaction. */
  onNew: () => void;
  /** `/`: open the command palette. */
  onSearch: () => void;
}

/**
 * Bare-key shortcuts for the app shell: `N` new transaction, `/` search and
 * `G` then `H/T/A/I/B/N` go-to. Skipped while typing in a field, while any
 * dialog/menu is open, and with ⌘/Ctrl/Alt held (so ⌘K and ⌘B are untouched).
 */
export function useShellShortcuts({ onNew, onSearch }: ShellShortcuts) {
  const router = useRouter();
  const handlers = useRef({ onNew, onSearch });
  useEffect(() => {
    handlers.current = { onNew, onSearch };
  });

  useEffect(() => {
    let armedAt = 0;

    const onKeyDown = (e: KeyboardEvent) => {
      if (shouldIgnoreShortcut(e)) {
        armedAt = 0;
        return;
      }
      const key = e.key.toLowerCase();

      if (armedAt && Date.now() - armedAt < CHORD_MS) {
        armedAt = 0;
        const dest = MAIN_NAV.find((n) => n.goKey === key);
        if (dest) {
          e.preventDefault();
          router.push(dest.href);
        }
        return;
      }
      armedAt = 0;

      if (key === "g") {
        armedAt = Date.now();
      } else if (key === "n") {
        e.preventDefault();
        handlers.current.onNew();
      } else if (key === "/") {
        e.preventDefault();
        handlers.current.onSearch();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [router]);
}
