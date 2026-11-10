import { describe, expect, it } from "vitest";
import { PRESET_COLORS } from "./category-colors";
import { PALETTES } from "./palettes";

function channel(hex: string, offset: number): number {
  const c = parseInt(hex.slice(offset, offset + 2), 16) / 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  return (
    0.2126 * channel(hex, 1) +
    0.7152 * channel(hex, 3) +
    0.0722 * channel(hex, 5)
  );
}

function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

describe("category colour presets", () => {
  it("has 8 to 10 distinct opaque hexes", () => {
    expect(PRESET_COLORS.length).toBeGreaterThanOrEqual(8);
    expect(PRESET_COLORS.length).toBeLessThanOrEqual(10);
    expect(new Set(PRESET_COLORS.map((c) => c.toLowerCase())).size).toBe(
      PRESET_COLORS.length
    );
    for (const c of PRESET_COLORS) expect(c).toMatch(/^#[0-9a-f]{6}$/i);
  });

  for (const palette of PALETTES) {
    for (const mode of ["light", "dark"] as const) {
      it(`reads as a tile fill (>= 3:1) on the ${palette.id} ${mode} card`, () => {
        const card = palette[mode].ink1;
        for (const color of PRESET_COLORS) {
          expect(
            contrast(color, card),
            `${color} on ${card}`
          ).toBeGreaterThanOrEqual(3);
        }
      });
    }
  }
});
