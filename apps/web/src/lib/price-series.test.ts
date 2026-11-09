import { describe, expect, it } from "vitest";
import {
  changePct,
  lastChangePct,
  priceRange,
  rangeDates,
  signedPercent,
  toPricePoints,
  tradeMarkers,
  weightPct,
  type PricePoint,
} from "./price-series";
import type { SecurityPrice } from "@/types/models";

function price(at: string, cents: number): SecurityPrice {
  return { id: at, security_id: "s", price: cents, recorded_at: at };
}

describe("toPricePoints", () => {
  it("sorts oldest first and keeps the last price of a day", () => {
    const pts = toPricePoints([
      price("2026-10-02T06:00:00Z", 200),
      price("2026-10-01T06:00:00Z", 100),
      price("2026-10-02T18:00:00Z", 250),
    ]);
    expect(pts).toEqual([
      { day: "2026-10-01", cents: 100 },
      { day: "2026-10-02", cents: 250 },
    ]);
  });
  it("ignores unparseable dates", () => {
    expect(toPricePoints([price("nope", 1)])).toEqual([]);
  });
});

describe("changes and range", () => {
  const pts: PricePoint[] = [
    { day: "2026-10-01", cents: 1000 },
    { day: "2026-10-02", cents: 800 },
    { day: "2026-10-03", cents: 1200 },
  ];
  it("computes the change over the series and over the last step", () => {
    expect(changePct(pts)).toBeCloseTo(20);
    expect(lastChangePct(pts)).toBeCloseTo(50);
  });
  it("is null without two points or from zero", () => {
    expect(changePct(pts.slice(0, 1))).toBeNull();
    expect(changePct([{ day: "a", cents: 0 }, { day: "b", cents: 5 }])).toBeNull();
  });
  it("finds the low and high", () => {
    expect(priceRange(pts)).toEqual({ low: 800, high: 1200 });
    expect(priceRange([])).toBeNull();
  });
});

describe("tradeMarkers", () => {
  const pts: PricePoint[] = [
    { day: "2026-10-01", cents: 1 },
    { day: "2026-10-03", cents: 1 },
    { day: "2026-10-05", cents: 1 },
  ];
  it("snaps trades to the next point and ignores dividends and old trades", () => {
    const m = tradeMarkers(
      [
        { type: "buy", date: "2026-10-02T00:00:00Z" },
        { type: "sell", date: "2026-10-09T00:00:00Z" },
        { type: "dividend", date: "2026-10-03T00:00:00Z" },
        { type: "buy", date: "2026-09-01T00:00:00Z" },
      ],
      pts
    );
    expect(m).toEqual([
      { day: "2026-10-03", tone: "pos" },
      { day: "2026-10-05", tone: "neg" },
    ]);
  });
  it("marks a day with a buy and a sell as neutral", () => {
    const m = tradeMarkers(
      [
        { type: "buy", date: "2026-10-03T00:00:00Z" },
        { type: "sell", date: "2026-10-03T00:00:00Z" },
      ],
      pts
    );
    expect(m).toEqual([{ day: "2026-10-03", tone: "neutral" }]);
  });
  it("is empty without points", () => {
    expect(tradeMarkers([{ type: "buy", date: "2026-10-03" }], [])).toEqual([]);
  });
});

describe("rangeDates", () => {
  it("spans the range and one day past today", () => {
    const r = rangeDates("3M", new Date("2026-11-09T12:00:00Z"));
    expect(r).toEqual({ from: "2026-08-09", to: "2026-11-10" });
  });
});

describe("signedPercent", () => {
  it("signs with a true minus and leaves zero unsigned", () => {
    expect(signedPercent(3.24)).toBe("+3.2%");
    expect(signedPercent(-0.44)).toBe("−0.4%");
    expect(signedPercent(0.01)).toBe("0.0%");
    expect(signedPercent(-84.76, 2)).toBe("−84.76%");
  });
});

describe("weightPct", () => {
  it("rounds to a whole percentage and guards a zero total", () => {
    expect(weightPct(1, 3)).toBe(33);
    expect(weightPct(5, 0)).toBe(0);
  });
});
