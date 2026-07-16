import {
  PALETTE,
  rgb,
  type DitherColor,
} from "@/components/dither-kit/palette";

/**
 * Ordered palette for categorical dither-kit series/slices. `grey` is reserved
 * for aggregate/empty buckets (e.g. an "Others" slice), so it is excluded here.
 */
export const DITHER_SERIES_COLORS: readonly DitherColor[] = [
  "green",
  "blue",
  "purple",
  "pink",
  "orange",
  "red",
] as const;

/** The dither-kit palette colour assigned to the nth categorical entry. */
export function ditherColorAt(index: number): DitherColor {
  return DITHER_SERIES_COLORS[index % DITHER_SERIES_COLORS.length];
}

/**
 * CSS `rgb()` string for a dither-kit palette colour's fill hue. Use for legend
 * dots and bars so DOM chrome matches the colour painted on the chart canvas.
 */
export function ditherFill(color: DitherColor): string {
  return rgb(PALETTE[color].fill);
}
