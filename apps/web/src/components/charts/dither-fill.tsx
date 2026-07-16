"use client";

import { useEffect, useRef, useState } from "react";
import type { AreaVariant } from "@/components/dither-kit/chart-context";
import {
  backingSize,
  bloomLayerStyle,
  paintColumn,
  prefersReducedMotion,
  type BloomInput,
} from "@/components/dither-kit/dither-paint";
import { PALETTE, type DitherColor } from "@/components/dither-kit/palette";

// Per-frame ease toward the hover-intensity target — matches the chart canvases.
const EASE = 0.16;

export type DitherFillProps = {
  /** Palette hue painted as the ordered-dither texture. */
  color: DitherColor;
  /** Fill texture — matches the chart series variants. */
  variant?: AreaVariant;
  /**
   * Denser fill with a raised floor — reads as a solid "filled" meter rather
   * than a chart series fading to its baseline. Mirrors dither-kit's stacked
   * paint mode.
   */
  solid?: boolean;
  /** Colour bloom shown at rest (same presets as the charts). */
  bloom?: BloomInput;
  /**
   * Brighten the fill (and fade in `hoverBloom`) while the bar is pointed at —
   * the same lift the chart canvases apply on hover. On by default.
   */
  hoverLift?: boolean;
  /** Bloom that fades in while hovered (when `hoverLift`). */
  hoverBloom?: BloomInput;
  /** Controlled hover; when omitted, the fill tracks its own parent element. */
  hovered?: boolean;
  className?: string;
};

/**
 * Absolutely fills its (positioned) parent with dither-kit's *own* ordered-
 * dither texture by driving the library's {@link paintColumn} painter — the
 * exact routine the area/bar/pie canvases use. Every column fades from a dense
 * base up to a bright border cap, and on hover the fill brightens + blooms with
 * the same eased `intensity` lift as the charts, so meters and progress bars
 * behave like the same material. The parent must be `relative` and clip
 * overflow; width/height come from the parent.
 */
export function DitherFill({
  color,
  variant = "gradient",
  solid = false,
  bloom = "off",
  hoverLift = true,
  hoverBloom = "low",
  hovered,
  className,
}: DitherFillProps) {
  const crispRef = useRef<HTMLCanvasElement>(null);
  const bloomRef = useRef<HTMLCanvasElement>(null);
  const intensityRef = useRef(0);
  const targetRef = useRef(0);
  const rafRef = useRef<number | null>(null);
  const ensureRef = useRef<() => void>(() => {});

  const [selfHover, setSelfHover] = useState(false);
  const isHovered = hoverLift && (hovered ?? selfHover);

  const restBloom = bloomLayerStyle(bloom, true);
  const liftBloom = hoverLift ? bloomLayerStyle(hoverBloom, true) : null;
  const hasBloom = Boolean(restBloom || liftBloom);

  // Track hover on the parent bar when the caller doesn't control it.
  useEffect(() => {
    if (hovered !== undefined || !hoverLift) return;
    const parent = crispRef.current?.parentElement;
    if (!parent) return;
    const enter = () => setSelfHover(true);
    const leave = () => setSelfHover(false);
    parent.addEventListener("pointerenter", enter);
    parent.addEventListener("pointerleave", leave);
    return () => {
      parent.removeEventListener("pointerenter", enter);
      parent.removeEventListener("pointerleave", leave);
    };
  }, [hovered, hoverLift]);

  // Paint machinery: renders the dither texture and eases the hover lift.
  useEffect(() => {
    const crisp = crispRef.current;
    const parent = crisp?.parentElement;
    if (!crisp || !parent) return;
    const seed = PALETTE[color];
    const reduce = prefersReducedMotion();

    const paintAt = (intensity: number) => {
      const ctx = crisp.getContext("2d");
      if (!ctx) return;
      const w = Math.max(1, Math.round(parent.clientWidth));
      const h = Math.max(1, Math.round(parent.clientHeight));
      const { cols, rows } = backingSize(w, h);
      crisp.width = cols;
      crisp.height = rows;
      ctx.clearRect(0, 0, cols, rows);
      // Paint the whole height as one dithered "bar": dense at the floor,
      // dissolving up to the bright cap — identical to a dither-kit bar column.
      for (let x = 0; x < cols; x++) {
        paintColumn(ctx, x, 0, rows, seed, {
          variant,
          intensity,
          dim: 1,
          stacked: solid,
        });
      }
      const bloomCanvas = bloomRef.current;
      if (bloomCanvas) {
        bloomCanvas.width = cols;
        bloomCanvas.height = rows;
        const bctx = bloomCanvas.getContext("2d");
        if (bctx) {
          bctx.clearRect(0, 0, cols, rows);
          bctx.drawImage(crisp, 0, 0);
        }
      }
    };

    const tick = () => {
      const target = targetRef.current;
      let i = intensityRef.current;
      const d = target - i;
      if (reduce || Math.abs(d) < 0.001) {
        i = target;
      } else {
        i += d * EASE;
      }
      intensityRef.current = i;
      paintAt(i);
      rafRef.current = i === target ? null : requestAnimationFrame(tick);
    };

    ensureRef.current = () => {
      if (rafRef.current == null) rafRef.current = requestAnimationFrame(tick);
    };

    paintAt(intensityRef.current);
    const ro = new ResizeObserver(() => paintAt(intensityRef.current));
    ro.observe(parent);
    return () => {
      ro.disconnect();
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    };
  }, [color, variant, solid, hasBloom]);

  // Drive the eased lift whenever the hover state flips.
  useEffect(() => {
    targetRef.current = isHovered ? 1 : 0;
    ensureRef.current();
  }, [isHovered]);

  const base = {
    position: "absolute",
    inset: 0,
    width: "100%",
    height: "100%",
    pointerEvents: "none",
  } as const;

  const bloomFilter = (liftBloom ?? restBloom)?.filter;
  const bloomBlend = (liftBloom ?? restBloom)?.mixBlendMode;
  const bloomOpacity = isHovered
    ? (liftBloom ?? restBloom)?.opacity ?? 0
    : restBloom?.opacity ?? 0;

  return (
    <>
      <canvas
        ref={crispRef}
        aria-hidden
        className={className}
        style={{ ...base, imageRendering: "pixelated" }}
      />
      {hasBloom && (
        <canvas
          ref={bloomRef}
          aria-hidden
          style={{
            ...base,
            imageRendering: "auto",
            filter: bloomFilter,
            mixBlendMode: bloomBlend,
            opacity: bloomOpacity,
            transition: "opacity 200ms ease",
          }}
        />
      )}
    </>
  );
}
