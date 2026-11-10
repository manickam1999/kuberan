import { describe, expect, it } from "vitest";
import {
  heatIntensity,
  monthBounds,
  rankedFlow,
  savingsRate,
  transactionsHref,
} from "./insights";
import type { Cashflow } from "@/types/api";

const item = (name: string, total: number, id: string | null = name) => ({
  category_id: id,
  category_name: name,
  category_color: "#123456",
  category_icon: "",
  total,
});

const flow = (
  expenses: ReturnType<typeof item>[],
  income: number
): Cashflow => ({
  income: [],
  expenses,
  total_income: income,
  total_expenses: expenses.reduce((s, e) => s + e.total, 0),
  from_date: "",
  to_date: "",
  currency: "MYR",
  fx_missing: false,
});

describe("transactionsHref", () => {
  it("lists only the filters given", () => {
    expect(transactionsHref({})).toBe("/transactions");
    expect(
      transactionsHref({
        categoryId: "c1",
        type: "expense",
        from: "2026-10-01",
        to: "2026-10-31",
      })
    ).toBe(
      "/transactions?category_id=c1&type=expense&from=2026-10-01&to=2026-10-31"
    );
    expect(transactionsHref({ categoryId: null, type: "income" })).toBe(
      "/transactions?type=income"
    );
  });
});

describe("monthBounds", () => {
  it("handles month lengths", () => {
    expect(monthBounds("2026-02")).toEqual({
      from: "2026-02-01",
      to: "2026-02-28",
    });
    expect(monthBounds("2028-02").to).toBe("2028-02-29");
    expect(monthBounds("2026-12").to).toBe("2026-12-31");
  });
});

describe("rankedFlow", () => {
  it("keeps the top categories, folds the rest and adds savings", () => {
    const rows = rankedFlow(
      flow(
        [item("A", 500), item("B", 400), item("C", 300), item("D", 100), item("E", 50)],
        2000
      ),
      3
    );
    expect(rows.map((r) => [r.label, r.amount])).toEqual([
      ["A", 500],
      ["B", 400],
      ["C", 300],
      ["Everything else", 150],
      ["Saved", 650],
    ]);
  });
  it("shows a deficit when spending outran income", () => {
    const rows = rankedFlow(flow([item("A", 500)], 200), 3);
    expect(rows[rows.length - 1]).toMatchObject({
      kind: "deficit",
      amount: 300,
    });
  });
});

describe("savingsRate", () => {
  it("is null without income", () => {
    expect(savingsRate(0, 100)).toBeNull();
    expect(savingsRate(1000, 750)).toBe(25);
    expect(savingsRate(1000, 1200)).toBe(-20);
  });
});

describe("heatIntensity", () => {
  it("is zero for no spending and grows with the amount", () => {
    expect(heatIntensity(0)).toBe(0);
    expect(heatIntensity(100)).toBeCloseTo(0.15);
    expect(heatIntensity(10000)).toBeGreaterThan(heatIntensity(1000));
    expect(heatIntensity(1e12)).toBe(1);
  });
});
