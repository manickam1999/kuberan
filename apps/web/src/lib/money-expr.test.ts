import { describe, expect, it } from "vitest";
import {
  AMOUNT_INPUT_PATTERN,
  evaluateAmount,
  evaluateAmountLenient,
  formatCentsPlain,
} from "./money-expr";

function cents(text: string): number | string {
  const r = evaluateAmount(text);
  return r.ok ? r.cents : r.reason;
}

describe("evaluateAmount values", () => {
  it.each([
    ["12", 1200],
    ["12.", 1200],
    [".5", 50],
    ["12.50", 1250],
    ["0", 0],
    ["0.1+0.2", 30],
    ["100/3", 3333],
    ["200/3", 6667],
    ["0.005", 1],
    ["-0.005", -1],
    ["0.004", 0],
    ["10/4", 250],
    ["1.075*2", 215],
  ])("%s -> %s", (input, expected) => {
    expect(cents(input)).toBe(expected);
  });

  it("respects precedence", () => {
    expect(cents("2+3*4")).toBe(1400);
    expect(cents("10-6/2")).toBe(700);
    expect(cents("10-2-3")).toBe(500);
    expect(cents("100/10/2")).toBe(500);
  });

  it("handles parentheses and unary signs", () => {
    expect(cents("(2+3)*4")).toBe(2000);
    expect(cents("((5))")).toBe(500);
    expect(cents("-5")).toBe(-500);
    expect(cents("-(2+3)")).toBe(-500);
    expect(cents("3*-2")).toBe(-600);
    expect(cents("12++3")).toBe(1500);
    expect(cents("+5")).toBe(500);
    expect(cents("5-3-4")).toBe(-200);
    expect(cents("0-0")).toBe(0);
  });

  it("strips commas and whitespace", () => {
    expect(cents("1,234.50")).toBe(123450);
    expect(cents(" 1 2 + 3 ")).toBe(1500);
  });

  it("accepts operator aliases", () => {
    expect(cents("3×4")).toBe(1200);
    expect(cents("3x4")).toBe(1200);
    expect(cents("3X4")).toBe(1200);
    expect(cents("12÷4")).toBe(300);
    expect(cents("5−2")).toBe(300);
    expect(cents("−5")).toBe(-500);
  });

  it("handles huge numbers", () => {
    expect(cents("99999999999999999999")).toBe("invalid");
    expect(cents("90071992547409.92")).toBe("invalid");
    expect(cents("90071992547409.91")).toBe(9007199254740991);
    expect(cents("-99999999999999999999")).toBe("invalid");
  });
});

describe("isExpression", () => {
  it.each([
    ["12", false],
    ["12.50", false],
    ["1,234.50", false],
    ["-5", false],
    ["5-3", true],
    ["(5)", true],
    ["2*3", true],
    ["3x4", true],
  ])("%s -> %s", (input, expected) => {
    const r = evaluateAmount(input);
    expect(r.ok && r.isExpression).toBe(expected);
  });
});

describe("failures", () => {
  it.each([
    ["", "empty"],
    ["   ", "empty"],
    [",", "invalid"],
    ["12+", "incomplete"],
    ["12*", "incomplete"],
    ["(12+3", "incomplete"],
    [".", "incomplete"],
    ["-", "incomplete"],
    ["(", "incomplete"],
    ["12)", "invalid"],
    ["1.2.3", "invalid"],
    ["()", "invalid"],
    ["5(3)", "invalid"],
    ["*5", "invalid"],
    ["5**3", "invalid"],
    ["abc", "invalid"],
    ["5/0", "div_zero"],
    ["5/(2-2)", "div_zero"],
    ["5/0.00", "div_zero"],
  ])("%s -> %s", (input, reason) => {
    expect(evaluateAmount(input)).toEqual({ ok: false, reason });
  });
});

describe("AMOUNT_INPUT_PATTERN", () => {
  it("accepts money characters and rejects others", () => {
    expect(AMOUNT_INPUT_PATTERN.test("1,234.5 + (2×3)÷4 x X −")).toBe(true);
    expect(AMOUNT_INPUT_PATTERN.test("12a")).toBe(false);
    expect(AMOUNT_INPUT_PATTERN.test("")).toBe(true);
  });
});

describe("formatCentsPlain", () => {
  it.each([
    [2140, "21.40"],
    [0, "0.00"],
    [-350, "-3.50"],
    [5, "0.05"],
    [-5, "-0.05"],
    [123456789, "1234567.89"],
  ])("%s -> %s", (input, expected) => {
    expect(formatCentsPlain(input)).toBe(expected);
  });
});

describe("thousands separators", () => {
  it.each([
    ["1,234.50", 123450],
    ["12,345,678", 1234567800],
    ["1,234+5", 123900],
    ["999,999.99", 99999999],
  ])("%s -> %s", (input, expected) => {
    expect(cents(input)).toBe(expected);
  });

  it.each([
    ["12,50", "invalid"],
    ["1,2", "invalid"],
    [",5", "invalid"],
    ["1,2345", "invalid"],
    ["1234,567", "invalid"],
    ["1,234.5,6", "invalid"],
    ["1,+2", "invalid"],
    ["1,", "incomplete"],
    ["1,234,", "incomplete"],
    ["2+1,", "incomplete"],
  ])("%s -> %s", (input, reason) => {
    expect(evaluateAmount(input)).toEqual({ ok: false, reason });
  });
});

describe("evaluateAmountLenient", () => {
  it.each([
    ["12+", 1200, false],
    ["(12+3", 1500, true],
    ["(4*(2+1", 1200, true],
    ["12*", 1200, false],
    ["12.", 1200, false],
    ["2+(", 200, false],
    ["1,", 100, false],
    ["12+3", 1500, true],
  ])("%s -> %s", (input, expected, isExpr) => {
    expect(evaluateAmountLenient(input)).toEqual({
      ok: true,
      cents: expected,
      isExpression: isExpr,
    });
  });

  it("keeps non-completable and non-incomplete results", () => {
    expect(evaluateAmountLenient("(")).toEqual({ ok: false, reason: "incomplete" });
    expect(evaluateAmountLenient("-")).toEqual({ ok: false, reason: "incomplete" });
    expect(evaluateAmountLenient("")).toEqual({ ok: false, reason: "empty" });
    expect(evaluateAmountLenient("12)")).toEqual({ ok: false, reason: "invalid" });
    expect(evaluateAmountLenient("5/0")).toEqual({ ok: false, reason: "div_zero" });
  });
});
