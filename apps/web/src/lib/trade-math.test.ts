import { describe, expect, it } from "vitest";
import {
  buyTotal,
  lineAmount,
  netDividend,
  previewSell,
  proportionalCost,
  validateTrade,
  type TradeFormValues,
} from "./trade-math";

describe("lineAmount", () => {
  it("rounds quantity x price to whole cents once", () => {
    expect(lineAmount(20, 18111)).toBe(362220);
    expect(lineAmount(0.5, 101)).toBe(51);
    expect(lineAmount(1 / 3, 100)).toBe(33);
  });
  it("is 0 without a quantity or price", () => {
    expect(lineAmount(0, 100)).toBe(0);
    expect(lineAmount(2, 0)).toBe(0);
  });
});

describe("proportionalCost", () => {
  it("removes a proportional share of the cost", () => {
    expect(proportionalCost(588000, 20, 60)).toBe(196000);
  });
  it("removes everything when the whole position is sold", () => {
    expect(proportionalCost(588001, 60, 60)).toBe(588001);
    expect(proportionalCost(588001, 60 - 1e-10, 60)).toBe(588001);
  });
  it("rounds half away from zero like Go's math.Round", () => {
    expect(proportionalCost(1, 1, 2)).toBe(1);
    expect(proportionalCost(3, 1, 2)).toBe(2);
  });
});

describe("previewSell", () => {
  const held = { position: 60, costBasis: 588000 };

  it("matches the prototype example: 20 of 60 at 181.11, fee 1.00", () => {
    const p = previewSell({ quantity: 20, pricePerUnit: 18111, fee: 100, ...held });
    expect(p).not.toBeNull();
    expect(p?.gross).toBe(362220);
    expect(p?.proceeds).toBe(362120);
    expect(p?.costSold).toBe(196000);
    expect(p?.realizedGain).toBe(166120);
    expect(p?.closesPosition).toBe(false);
    expect(p?.realizedPct).toBeCloseTo(84.76, 1);
  });

  it("reports a loss with a negative gain", () => {
    const p = previewSell({ quantity: 60, pricePerUnit: 5000, fee: 0, ...held });
    expect(p?.realizedGain).toBe(300000 - 588000);
    expect(p?.closesPosition).toBe(true);
  });

  it("is null for no quantity, no price, or more than is held", () => {
    expect(previewSell({ quantity: 0, pricePerUnit: 100, fee: 0, ...held })).toBeNull();
    expect(previewSell({ quantity: 1, pricePerUnit: 0, fee: 0, ...held })).toBeNull();
    expect(previewSell({ quantity: 61, pricePerUnit: 100, fee: 0, ...held })).toBeNull();
  });

  it("tolerates float dust on a full sale", () => {
    const p = previewSell({ quantity: 60 + 1e-10, pricePerUnit: 100, fee: 0, ...held });
    expect(p?.closesPosition).toBe(true);
    expect(p?.costSold).toBe(588000);
  });
});

describe("buyTotal and netDividend", () => {
  it("adds the fee to the purchase", () => {
    expect(buyTotal(10, 1000, 150)).toBe(10150);
    expect(buyTotal(0, 1000, 150)).toBe(0);
  });
  it("deducts withholding from a dividend, never below zero", () => {
    expect(netDividend(1000, 150)).toBe(850);
    expect(netDividend(100, 150)).toBe(0);
  });
});

describe("validateTrade", () => {
  const base: TradeFormValues = {
    kind: "buy",
    date: "2026-11-01",
    quantity: 1,
    pricePerUnit: 100,
    fee: 0,
    amount: 0,
    splitRatio: 0,
    position: 10,
  };
  it("accepts a complete buy", () => {
    expect(validateTrade(base)).toBeNull();
  });
  it("requires a date first", () => {
    expect(validateTrade({ ...base, date: "" })).toBe("Date is required");
  });
  it("rejects selling more than held", () => {
    expect(validateTrade({ ...base, kind: "sell", quantity: 11 })).toMatch(
      /Cannot sell more than you hold/
    );
    expect(validateTrade({ ...base, kind: "sell", quantity: 10 })).toBeNull();
  });
  it("checks dividend amount and withholding", () => {
    expect(validateTrade({ ...base, kind: "dividend", amount: 0 })).toBe(
      "Amount must be greater than zero"
    );
    expect(validateTrade({ ...base, kind: "dividend", amount: 100, fee: 101 })).toMatch(
      /cannot exceed/
    );
    expect(validateTrade({ ...base, kind: "dividend", amount: 100, fee: 15 })).toBeNull();
  });
  it("checks the split ratio", () => {
    expect(validateTrade({ ...base, kind: "split" })).toBe(
      "Split ratio must be greater than zero"
    );
    expect(validateTrade({ ...base, kind: "split", splitRatio: 2 })).toBeNull();
  });
});
