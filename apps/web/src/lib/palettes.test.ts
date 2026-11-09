import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  BUILT_IN_PALETTE,
  PALETTES,
  getPalette,
  isPaletteId,
  type PaletteTokens,
} from "./palettes";

// --- WCAG 2.x contrast -------------------------------------------------------

function channel(hex: string, offset: number): number {
  const c = parseInt(hex.slice(offset, offset + 2), 16) / 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  if (!/^#[0-9a-f]{6}$/i.test(hex)) {
    throw new Error(`expected an opaque #rrggbb colour, got ${hex}`);
  }
  return (
    0.2126 * channel(hex, 1) +
    0.7152 * channel(hex, 3) +
    0.0722 * channel(hex, 5)
  );
}

/** WCAG contrast ratio between two opaque hex colours. */
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe("contrast helper", () => {
  it("matches the WCAG reference values", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrast("#777777", "#ffffff")).toBeCloseTo(4.48, 2);
  });
});

// --- Rules -------------------------------------------------------------------

const TEXT_RATIO = 7; // primary text on background and card
const SECONDARY_RATIO = 4.5; // secondary text, positive and negative

interface Check {
  name: string;
  fg: keyof PaletteTokens;
  bg: keyof PaletteTokens;
  min: number;
}

const SURFACES = ["ink0", "ink1"] as const; // background, card
const CHECKS: Check[] = SURFACES.flatMap((bg) => [
  { name: `text on ${bg}`, fg: "fg" as const, bg, min: TEXT_RATIO },
  {
    name: `secondary text on ${bg}`,
    fg: "fg2" as const,
    bg,
    min: SECONDARY_RATIO,
  },
  { name: `positive on ${bg}`, fg: "pos" as const, bg, min: SECONDARY_RATIO },
  { name: `negative on ${bg}`, fg: "neg" as const, bg, min: SECONDARY_RATIO },
]);

/**
 * Pairs that miss the rule above (all four prototype misses were fixed in 2.1;
 * keep this empty). A new entry must be reported, not silently re-coloured;
 * the test fails when an entry stops failing, so the list
 * cannot go stale.
 */
const KNOWN_PROTOTYPE_FAILURES = new Set<string>([]);

describe.each(PALETTES)("palette $id contrast", (palette) => {
  for (const mode of ["light", "dark"] as const) {
    for (const check of CHECKS) {
      const key = `${palette.id}/${mode}: ${check.name}`;
      const known = KNOWN_PROTOTYPE_FAILURES.has(key);
      it(`${mode}: ${check.name} >= ${check.min}:1${known ? " (known prototype miss)" : ""}`, () => {
        const tokens = palette[mode];
        const ratio = contrast(tokens[check.fg], tokens[check.bg]);
        if (known) {
          expect(ratio, `${key} now passes; remove it from the list`).toBeLessThan(
            check.min
          );
        } else {
          expect(ratio, key).toBeGreaterThanOrEqual(check.min);
        }
      });
    }
  }
});

// --- Registry ----------------------------------------------------------------

describe("registry", () => {
  it("has the eight prototype palettes with Catppuccin built in", () => {
    expect(PALETTES.map((p) => p.id)).toEqual([
      "gold",
      "cobalt",
      "saffron",
      "graphite",
      "lagoon",
      "nord",
      "catppuccin",
      "gruvbox",
    ]);
    expect(BUILT_IN_PALETTE).toBe("catppuccin");
  });

  it("validates ids and falls back to the built-in palette", () => {
    expect(isPaletteId("nord")).toBe(true);
    expect(isPaletteId("default")).toBe(false);
    expect(isPaletteId(undefined)).toBe(false);
    expect(getPalette("nope").id).toBe("catppuccin");
    expect(getPalette("gruvbox").name).toBe("Gruvbox");
  });
});

// --- CSS mirrors the registry ------------------------------------------------

const css = readFileSync(join(__dirname, "../app/globals.css"), "utf8");

/** The declarations of the rule whose selector is exactly `selector`. */
function ruleBody(selector: string): string {
  const start = css.indexOf(`\n${selector} {`);
  if (start < 0) throw new Error(`no rule for ${selector}`);
  return css.slice(start, css.indexOf("\n}", start));
}

function declared(body: string, name: string): string {
  const m = body.match(new RegExp(`\\n\\s*${name}:\\s*([^;]+);`));
  if (!m) throw new Error(`${name} missing`);
  return m[1].trim();
}

describe("globals.css mirrors the registry", () => {
  const cases: { selector: string; mode: "light" | "dark"; id: string }[] = [
    { selector: ":root", mode: "light", id: BUILT_IN_PALETTE },
    { selector: ".dark", mode: "dark", id: BUILT_IN_PALETTE },
    ...PALETTES.flatMap((p) => [
      { selector: `[data-palette="${p.id}"]`, mode: "light" as const, id: p.id },
      {
        selector: `.dark[data-palette="${p.id}"]`,
        mode: "dark" as const,
        id: p.id,
      },
    ]),
  ];

  it.each(cases)("$selector", ({ selector, mode, id }) => {
    const t = getPalette(id)[mode];
    const body = ruleBody(selector);
    expect(declared(body, "--background")).toBe(t.ink0);
    expect(declared(body, "--card")).toBe(t.ink1);
    expect(declared(body, "--elevated")).toBe(t.ink2);
    expect(declared(body, "--muted")).toBe(t.ink3);
    expect(declared(body, "--foreground")).toBe(t.fg);
    expect(declared(body, "--muted-foreground")).toBe(t.fg2);
    expect(declared(body, "--primary")).toBe(t.accent);
    expect(declared(body, "--primary-foreground")).toBe(t.accentInk);
    expect(declared(body, "--positive")).toBe(t.pos);
    expect(declared(body, "--negative")).toBe(t.neg);
    expect(declared(body, "--warning")).toBe(t.warn);
    expect(declared(body, "--border")).toBe(t.line);
    expect(declared(body, "--chart-1")).toBe(t.k1);
    expect(declared(body, "--chart-6")).toBe(t.k6);
    expect(declared(body, "color-scheme")).toBe(mode);
  });
});
