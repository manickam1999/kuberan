import { describe, expect, it } from "vitest";
import {
  MASKED_AMOUNT,
  MINUS_SIGN,
  formatMoney,
  formatMoneyParts,
} from "./format";

/** Collapse any space-like character (NBSP, narrow NBSP) to a plain space. */
const norm = (s: string) => s.replace(/[\s  ]/g, " ");

describe("formatMoney", () => {
  it("uses U+2212 as the minus sign", () => {
    expect(MINUS_SIGN).toBe("−");
  });

  it.each([
    ["MYR", 123450, "RM 1,234.50", -6473, "RM −64.73"],
    ["USD", 123450, "US$1,234.50", -500, "US$−5.00"],
    ["EUR", 123450, "€1,234.50", -500, "€−5.00"],
    ["SGD", 123450, "SGD 1,234.50", -500, "SGD −5.00"],
  ])("%s positive and negative", (currency, pos, posText, neg, negText) => {
    expect(norm(formatMoney(pos, currency))).toBe(posText);
    expect(norm(formatMoney(neg, currency))).toBe(negText);
  });

  it("never emits an ASCII hyphen", () => {
    for (const c of ["MYR", "USD", "EUR", "SGD"]) {
      expect(formatMoney(-12345, c)).not.toContain("-");
    }
  });

  it("formats zero without a sign", () => {
    expect(norm(formatMoney(0, "MYR"))).toBe("RM 0.00");
  });

  it("masks regardless of sign", () => {
    expect(formatMoney(-6473, "MYR", true)).toBe(MASKED_AMOUNT);
    expect(formatMoney(6473, "USD", true)).toBe(MASKED_AMOUNT);
  });
});

describe("formatMoneyParts", () => {
  it("splits symbol, minus, integer and fraction", () => {
    expect(formatMoneyParts(-123456, "USD")).toEqual({
      prefix: "US$",
      minus: MINUS_SIGN,
      integer: "1,234",
      fraction: ".56",
      suffix: "",
    });
  });

  it("supports compact notation", () => {
    const p = formatMoneyParts(123456789, "USD", true);
    expect(p.prefix).toBe("US$");
    expect(p.integer).toBe("1");
    expect(p.fraction).toContain("2");
  });
});

describe("compact money below 1,000", () => {
  it("shows whole units instead of one stray decimal", () => {
    expect(formatMoneyParts(28190, "MYR", true).fraction).toBe("");
    expect(formatMoneyParts(28190, "MYR", true).integer).toBe("282");
    expect(formatMoneyParts(123456, "MYR", true).fraction).toMatch(/K/);
  });
});
