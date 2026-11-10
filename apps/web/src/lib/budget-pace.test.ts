import { describe, expect, it } from "vitest";
import {
  budgetStatus,
  paceDelta,
  periodDaysLeft,
  periodElapsedFraction,
} from "./budget-pace";

describe("periodElapsedFraction", () => {
  it("is the day of month over the month length", () => {
    expect(periodElapsedFraction("monthly", new Date(2026, 9, 31))).toBe(1);
    expect(periodElapsedFraction("monthly", new Date(2026, 1, 14))).toBe(0.5);
    expect(periodElapsedFraction("monthly", new Date(2026, 0, 1))).toBeCloseTo(
      1 / 31
    );
  });
  it("handles leap years for yearly budgets", () => {
    expect(periodElapsedFraction("yearly", new Date(2028, 11, 31))).toBe(1);
    expect(periodElapsedFraction("yearly", new Date(2026, 0, 1))).toBeCloseTo(
      1 / 365
    );
    expect(periodElapsedFraction("yearly", new Date(2028, 0, 1))).toBeCloseTo(
      1 / 366
    );
  });
});

describe("periodDaysLeft", () => {
  it("counts days after today", () => {
    expect(periodDaysLeft("monthly", new Date(2026, 9, 31))).toBe(0);
    expect(periodDaysLeft("monthly", new Date(2026, 9, 1))).toBe(30);
    expect(periodDaysLeft("yearly", new Date(2026, 11, 31))).toBe(0);
    expect(periodDaysLeft("yearly", new Date(2026, 11, 30))).toBe(1);
  });
});

describe("budgetStatus", () => {
  it("classifies by percentage", () => {
    expect(budgetStatus(0)).toBe("on_track");
    expect(budgetStatus(79.9)).toBe("on_track");
    expect(budgetStatus(80)).toBe("close");
    expect(budgetStatus(100)).toBe("close");
    expect(budgetStatus(100.1)).toBe("over");
  });
});

describe("paceDelta", () => {
  it("is spent minus the even-pace amount", () => {
    expect(paceDelta(100000, 60000, 0.5)).toBe(10000);
    expect(paceDelta(100000, 30000, 0.5)).toBe(-20000);
  });
});
