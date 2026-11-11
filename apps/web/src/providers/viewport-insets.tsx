"use client";

import { useEffect } from "react";
import { keyboardInset } from "@/lib/keyboard-inset";

/**
 * Publishes the on-screen keyboard's height as `--kb-inset` and the visible
 * height as `--vv-height` on <html>, so bottom sheets can sit on top of the
 * keyboard instead of behind it (see the dialog sheet in globals.css).
 */
export function ViewportInsets() {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const root = document.documentElement;
    let frame = 0;

    const update = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const inset = keyboardInset(window.innerHeight, vv);
        root.style.setProperty("--kb-inset", `${inset}px`);
        root.style.setProperty("--vv-height", `${Math.round(vv.height)}px`);
        root.toggleAttribute("data-keyboard", inset > 0);
      });
    };

    update();
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    return () => {
      cancelAnimationFrame(frame);
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
      root.style.removeProperty("--kb-inset");
      root.style.removeProperty("--vv-height");
      root.removeAttribute("data-keyboard");
    };
  }, []);

  return null;
}
