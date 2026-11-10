import { describe, expect, it } from "vitest";
import { holdingFigures, holdingReturnPct } from "./holding-math";
import type { Investment } from "@/types/models";

function holding(overrides: Partial<Investment>): Investment {
  return {
    quantity: 60,
    cost_basis: 588000, // US$5,880.00
    cost_basis_base: 2528400, // RM 25,284.00
    current_price: 81500, // RM 815.00 per unit
    current_price_native: 18111, // US$181.11
    price_currency: "USD",
    currency: "USD",
    fx_missing: false,
    ...overrides,
  } as Investment;
}

describe("holdingFigures", () => {
  it("keeps native and base figures apart", () => {
    const f = holdingFigures(holding({}));
    expect(f.currency).toBe("USD");
    expect(f.nativeValue).toBe(1086660);
    expect(f.nativePnl).toBe(1086660 - 588000);
    expect(f.baseValue).toBe(4890000);
    expect(f.basePnl).toBe(4890000 - 2528400);
  });

  it("has no base figures without an exchange rate", () => {
    const f = holdingFigures(holding({ fx_missing: true, current_price: 0 }));
    expect(f.baseValue).toBeNull();
    expect(f.basePnl).toBeNull();
    expect(f.nativePnl).toBe(1086660 - 588000);
  });
});

describe("holdingReturnPct", () => {
  it("is the base gain over the base cost when there is a rate", () => {
    expect(holdingReturnPct(holding({}))).toBeCloseTo(93.4, 1);
  });

  it("falls back to the native gain over the native cost without a rate", () => {
    expect(
      holdingReturnPct(holding({ fx_missing: true, current_price: 0 }))
    ).toBeCloseTo(84.8, 1);
  });

  it("is 0 for a holding with no cost", () => {
    expect(
      holdingReturnPct(holding({ cost_basis: 0, cost_basis_base: 0 }))
    ).toBe(0);
  });
});
