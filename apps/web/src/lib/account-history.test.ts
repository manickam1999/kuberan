import { describe, expect, it } from "vitest";
import {
  historyChange,
  historyChartData,
  historyWindow,
  utcDay,
} from "./account-history";

describe("historyWindow", () => {
  it("covers the last N days ending today", () => {
    const today = new Date("2026-11-09T15:30:00Z");
    expect(historyWindow(30, today)).toEqual({
      from: "2026-10-11",
      to: "2026-11-09",
    });
    expect(historyWindow(1, today)).toEqual({
      from: "2026-11-09",
      to: "2026-11-09",
    });
  });

  it("formats a UTC day", () => {
    expect(utcDay(new Date("2026-01-02T23:59:59Z"))).toBe("2026-01-02");
  });
});

describe("historyChange", () => {
  it("needs two points", () => {
    expect(historyChange([])).toBeNull();
    expect(historyChange([{ balance: 5 }])).toBeNull();
  });

  it("returns the move and its percent", () => {
    expect(historyChange([{ balance: 1000 }, { balance: 1250 }])).toEqual({
      delta: 250,
      pct: 25,
    });
    expect(historyChange([{ balance: 1000 }, { balance: 500 }])).toEqual({
      delta: -500,
      pct: -50,
    });
  });

  it("has no percent from a zero start", () => {
    expect(historyChange([{ balance: 0 }, { balance: 100 }])).toEqual({
      delta: 100,
      pct: null,
    });
  });
});

describe("historyChartData", () => {
  it("labels days and converts cents", () => {
    expect(
      historyChartData([
        { day: "2026-11-03", balance: 12345 },
        { day: "2026-11-04", balance: 0 },
      ]),
    ).toEqual([
      { label: "Nov 3", value: 123.45 },
      { label: "Nov 4", value: 0 },
    ]);
  });
});
