"use client";

import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

/** DOM id of the header's desktop slot (right after the sidebar trigger). */
export const HEADER_SLOT_ID = "app-header-slot";

/**
 * Renders `children` into the app header's desktop slot, e.g. a month
 * switcher. Renders nothing until the header has mounted, and nothing on
 * phones (the slot is hidden below `md`; pages keep their own control there).
 */
export function HeaderSlot({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<HTMLElement | null>(null);

  useEffect(() => {
    // The slot lives in the layout's header, outside this component's tree.
    setTarget(document.getElementById(HEADER_SLOT_ID));
  }, []);

  return target ? createPortal(children, target) : null;
}
