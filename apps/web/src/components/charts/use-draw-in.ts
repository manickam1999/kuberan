"use client";

import { useLayoutEffect, type RefObject } from "react";

/** How long the line takes to draw in, in ms. */
export const DRAW_MS = 1400;
/** Frames to wait for Recharts to render the curve before giving up. */
const MAX_WAIT_FRAMES = 60;

/** Class names ThemedAreaChart puts on the parts this hook animates. */
export const DRAW_CLASS = {
  /** The main series' Area layer: revealed left to right. */
  area: "chart-draw",
  /** Endpoint dot and halo: ride the tip of the line. */
  end: "chart-end",
  /** Trade markers: appear as the tip passes them. */
  marker: "chart-marker",
} as const;

const easeOutCubic = (t: number) => 1 - (1 - t) ** 3;

/** Point on `path` whose x is `x` (the curve runs left to right). */
function pointAtX(path: SVGPathElement, total: number, x: number) {
  let lo = 0;
  let hi = total;
  let p = path.getPointAtLength(0);
  for (let i = 0; i < 18; i++) {
    const mid = (lo + hi) / 2;
    p = path.getPointAtLength(mid);
    if (p.x < x) lo = mid;
    else hi = mid;
  }
  return p;
}

/**
 * Draws the area chart in from the left with the endpoint dot riding the tip
 * of the line, on mount and whenever `drawKey` changes. Recharts' own
 * animation must be off for the series (`isAnimationActive={false}`).
 *
 * While waiting for Recharts to render, `data-drawing="pending"` on the
 * container hides the series (see globals.css), so it never flashes in full.
 * Skipped under prefers-reduced-motion; everything is shown as rendered.
 */
export function useDrawIn(
  ref: RefObject<HTMLElement | null>,
  drawKey: string,
  enabled: boolean
) {
  useLayoutEffect(() => {
    const root = ref.current;
    if (!root || !enabled || !drawKey) return;
    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;

    const q = <T extends Element>(sel: string) =>
      Array.from(root.querySelectorAll<T>(sel));

    root.dataset.drawing = "pending";
    let frame = 0;
    let waited = 0;
    let start: number | null = null;

    const reset = () => {
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.area}`)) {
        g.style.removeProperty("clip-path");
      }
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.end}`)) {
        g.style.removeProperty("transform");
      }
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.marker}`)) {
        g.style.removeProperty("opacity");
      }
      delete root.dataset.drawing;
    };

    const step = (now: number) => {
      const curve = root.querySelector<SVGPathElement>(
        `.${DRAW_CLASS.area} .recharts-area-curve`
      );
      // Looked up each frame: on first mount Recharts has not rendered yet.
      const clip = root.querySelector("[data-draw-clip]");
      const clipRect = clip?.querySelector("rect");
      const clipId = clip?.getAttribute("id");
      const total = curve?.getTotalLength() ?? 0;
      if (!curve || !clipRect || !clipId || total === 0) {
        // Not rendered or laid out yet (ResponsiveContainer measures first).
        if (++waited > MAX_WAIT_FRAMES) {
          reset();
          return;
        }
        frame = requestAnimationFrame(step);
        return;
      }

      start ??= now;
      const t = Math.min(1, (now - start) / DRAW_MS);
      const box = curve.getBBox();
      const tipX = box.x + box.width * easeOutCubic(t);
      const tip = pointAtX(curve, total, tipX);
      const end = curve.getPointAtLength(total);

      clipRect.setAttribute("width", String(Math.max(0, tipX + 10000)));
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.area}`)) {
        g.style.clipPath = `url(#${clipId})`;
      }
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.end}`)) {
        g.style.transform = `translate(${tip.x - end.x}px, ${tip.y - end.y}px)`;
      }
      for (const g of q<SVGGElement>(`.${DRAW_CLASS.marker}`)) {
        const cx = Number(g.querySelector("circle")?.getAttribute("cx"));
        g.style.opacity = cx <= tipX + 0.5 ? "1" : "0";
      }
      root.dataset.drawing = "running";

      if (t < 1) frame = requestAnimationFrame(step);
      else reset();
    };

    frame = requestAnimationFrame(step);
    return () => {
      cancelAnimationFrame(frame);
      reset();
    };
  }, [ref, drawKey, enabled]);
}
