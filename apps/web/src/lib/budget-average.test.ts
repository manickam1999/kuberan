import { describe, expect, it } from "vitest";
import { lastSixMonthsRange, sixMonthAverage } from "./budget-average";

describe("lastSixMonthsRange", () => {
  it("covers the six whole months before the current one", () => {
    expect(lastSixMonthsRange(new Date(2026, 10, 9))).toEqual({
      from: "2026-05-01",
      to: "2026-10-31",
    });
  });
  it("crosses the year boundary", () => {
    expect(lastSixMonthsRange(new Date(2026, 2, 15))).toEqual({
      from: "2025-09-01",
      to: "2026-02-28",
    });
  });
});

describe("sixMonthAverage", () => {
  it("rounds to the nearest cent", () => {
    expect(sixMonthAverage(600)).toBe(100);
    expect(sixMonthAverage(1000)).toBe(167);
    expect(sixMonthAverage(0)).toBe(0);
  });
});
