import { describe, expect, it } from "vitest";
import {
  MONEY_SIZE_CLASS,
  hasRaisedCurrency,
  moneyDisplay,
  moneyText,
} from "./money-display";

const norm = (s: string) => s.replace(/[\s  ]/g, " ");

describe("moneyDisplay sign modes", () => {
  it("auto: minus only when negative", () => {
    expect(moneyDisplay(-500, "USD").sign).toBe("−");
    expect(moneyDisplay(500, "USD").sign).toBe("");
    expect(moneyDisplay(0, "USD").sign).toBe("");
  });

  it("always: plus for positive, minus for negative, none for zero", () => {
    expect(moneyDisplay(500, "USD", "always").sign).toBe("+");
    expect(moneyDisplay(-500, "USD", "always").sign).toBe("−");
    expect(moneyDisplay(0, "USD", "always").sign).toBe("");
  });

  it("never: magnitude only", () => {
    expect(moneyDisplay(-500, "USD", "never").sign).toBe("");
    expect(norm(moneyText(-6473, "MYR", "never"))).toBe("RM 64.73");
  });
});

describe("moneyText", () => {
  it.each([
    ["MYR", -6473, "auto", "RM −64.73"],
    ["USD", 500, "always", "US$+5.00"],
    ["EUR", -500, "auto", "€−5.00"],
    ["SGD", 123450, "auto", "SGD 1,234.50"],
  ] as const)("%s %i %s", (currency, cents, sign, expected) => {
    expect(norm(moneyText(cents, currency, sign))).toBe(expected);
  });

  it("masks", () => {
    expect(moneyText(-500, "USD", "auto", false, true)).toBe("****");
  });
});

describe("size", () => {
  it("figure sizes use the figures face, sm does not", () => {
    for (const s of ["hero", "lg", "md"] as const) {
      expect(MONEY_SIZE_CLASS[s]).toContain("font-figures");
      expect(hasRaisedCurrency(s)).toBe(true);
    }
    expect(MONEY_SIZE_CLASS.sm).not.toContain("font-figures");
    expect(hasRaisedCurrency("sm")).toBe(false);
    expect(hasRaisedCurrency(undefined)).toBe(false);
  });
});
