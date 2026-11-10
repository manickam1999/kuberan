import { describe, expect, it } from "vitest";
import type { TypicalMonth } from "@/types/api";
import {
  categoryComparison,
  hasTypical,
  paceSeries,
  typicalAtDay,
  typicalScale,
  typicalSpendFor,
  vsTypicalLabel,
  vsTypicalPercent,
  vsTypicalTone,
} from "./typical";

const curve = Array.from({ length: 31 }, (_, i) => (i + 1) * 100);

const typical: TypicalMonth = {
  months: 6,
  months_used: 3,
  currency: "MYR",
  avg_income: 500000,
  avg_spending: 3100,
  saved_percent: 40,
  daily_cumulative: curve,
  by_category: [
    {
      category_id: "food",
      category_name: "Food",
      category_color: "",
      category_icon: "",
      total: 2000,
    },
    {
      category_id: null,
      category_name: "Uncategorized",
      category_color: "",
      category_icon: "",
      total: 1100,
    },
  ],
  fx_missing: false,
  unconverted: [],
};

describe("hasTypical", () => {
  it("needs at least one month of history", () => {
    expect(hasTypical(typical)).toBe(true);
    expect(hasTypical({ ...typical, months_used: 0 })).toBe(false);
    expect(hasTypical(undefined)).toBe(false);
  });
});

describe("typicalAtDay", () => {
  it("reads the 1-based day and clamps", () => {
    expect(typicalAtDay(curve, 1)).toBe(100);
    expect(typicalAtDay(curve, 10)).toBe(1000);
    expect(typicalAtDay(curve, 0)).toBe(100);
    expect(typicalAtDay(curve, 99)).toBe(3100);
    expect(typicalAtDay([], 5)).toBe(0);
  });
});

describe("vsTypical", () => {
  it("computes the rounded percent difference", () => {
    expect(vsTypicalPercent(820, 1000)).toBe(-18);
    expect(vsTypicalPercent(1125, 1000)).toBe(13);
    expect(vsTypicalPercent(1000, 1000)).toBe(0);
  });

  it("has no answer without a typical amount", () => {
    expect(vsTypicalPercent(500, 0)).toBeNull();
  });

  it("labels and tones", () => {
    expect(vsTypicalLabel(-18)).toBe("−18% vs typical");
    expect(vsTypicalLabel(12)).toBe("+12% vs typical");
    expect(vsTypicalLabel(0)).toBe("On par with typical");
    expect(vsTypicalTone(-5)).toBe("pos");
    expect(vsTypicalTone(5)).toBe("neg");
    expect(vsTypicalTone(0)).toBe("default");
    expect(vsTypicalTone(null)).toBe("default");
  });
});

describe("typicalSpendFor / typicalScale", () => {
  const today = new Date(2026, 10, 10); // 10 Nov 2026

  it("uses the full average for a past month", () => {
    expect(typicalSpendFor(typical, "2026-10-01", today)).toBe(3100);
    expect(typicalScale(typical, "2026-10-01", today)).toBe(1);
  });

  it("uses the pace up to today for the current month", () => {
    expect(typicalSpendFor(typical, "2026-11-01", today)).toBe(1000);
    expect(typicalScale(typical, "2026-11-01", today)).toBeCloseTo(1000 / 3100);
  });

  it("has nothing for a future month or no history", () => {
    expect(typicalSpendFor(typical, "2026-12-01", today)).toBeNull();
    expect(typicalScale(typical, "2026-12-01", today)).toBeNull();
    expect(
      typicalSpendFor({ ...typical, months_used: 0 }, "2026-10-01", today),
    ).toBeNull();
  });
});

describe("categoryComparison", () => {
  it("scales typical amounts and flags categories without history", () => {
    const items = [
      { ...typical.by_category[0], total: 1500 },
      { ...typical.by_category[1], total: 1100 },
      {
        category_id: "new",
        category_name: "New",
        category_color: "",
        category_icon: "",
        total: 400,
      },
    ];
    const cmp = categoryComparison(items, typical, 0.5);
    expect(cmp.get("food")).toEqual({ typical: 1000, pct: 50 });
    expect(cmp.get("uncategorized")).toEqual({ typical: 550, pct: 100 });
    expect(cmp.get("new")).toEqual({ typical: 0, pct: null });
  });
});

describe("paceSeries", () => {
  it("cuts both lines at the given day", () => {
    const out = paceSeries([50, 80, 80, 200], curve, 3);
    expect(out.actual).toEqual([50, 80, 80]);
    expect(out.typical).toEqual([100, 200, 300]);
  });

  it("never reaches past the actual data", () => {
    expect(paceSeries([10, 20], curve, 31).typical).toEqual([100, 200]);
    expect(paceSeries([], curve, 5)).toEqual({ actual: [], typical: [] });
  });
});
