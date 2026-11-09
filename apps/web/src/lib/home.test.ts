import { describe, expect, it } from "vitest";
import {
  balanceSheet,
  budgetStatus,
  cumulative,
  dayLabel,
  groupByDay,
  needsOnboarding,
  onboardingSteps,
} from "./home";

describe("balanceSheet", () => {
  it("adds fixed deposits to cash and splits the asset bar", () => {
    const b = balanceSheet({
      cash_balance: 3000,
      fixed_deposit_balance: 1000,
      investment_value: 4000,
      debt_balance: 500,
    });
    expect(b.cash).toBe(4000);
    expect(b.debt).toBe(500);
    expect(b.cashPct).toBe(50);
  });

  it("handles no assets", () => {
    expect(
      balanceSheet({ cash_balance: 0, investment_value: 0, debt_balance: 9 })
        .cashPct
    ).toBe(0);
  });
});

describe("onboardingSteps", () => {
  const base = {
    accounts: [] as { id: string }[],
    transactions: [] as { type: "income" | "opening_balance" }[],
    budgetCount: 0,
    connectedAppCount: 0,
  };

  it("starts with nothing done", () => {
    const steps = onboardingSteps(base);
    expect(steps.map((s) => s.done)).toEqual([false, false, false, false]);
    expect(needsOnboarding(steps)).toBe(true);
  });

  it("does not count an opening balance as a transaction", () => {
    const steps = onboardingSteps({
      ...base,
      accounts: [{ id: "a" }],
      transactions: [{ type: "opening_balance" }],
    });
    expect(steps.map((s) => s.done)).toEqual([true, false, false, false]);
    expect(needsOnboarding(steps)).toBe(true);
  });

  it("leaves onboarding once an account has real activity", () => {
    const steps = onboardingSteps({
      ...base,
      accounts: [{ id: "a" }],
      transactions: [{ type: "income" }],
    });
    expect(needsOnboarding(steps)).toBe(false);
  });
});

describe("day grouping", () => {
  const now = new Date(2026, 10, 9, 12);
  it("labels today, yesterday and older days", () => {
    expect(dayLabel("2026-11-09T00:00:00Z", now)).toBe("Today");
    expect(dayLabel("2026-11-08T00:00:00Z", now)).toBe("Yesterday");
    expect(dayLabel("2026-10-27T00:00:00Z", now)).toBe("Tue 27 Oct");
  });

  it("groups consecutive rows of one day", () => {
    const g = groupByDay(
      [
        { date: "2026-11-09T00:00:00Z" },
        { date: "2026-11-09T00:00:00Z" },
        { date: "2026-11-08T00:00:00Z" },
      ],
      now
    );
    expect(g.map((x) => x.items.length)).toEqual([2, 1]);
  });
});

describe("budgetStatus", () => {
  it("maps percentages", () => {
    expect(budgetStatus(40)).toBe("ok");
    expect(budgetStatus(75)).toBe("close");
    expect(budgetStatus(100)).toBe("close");
    expect(budgetStatus(100.5)).toBe("over");
  });
});

describe("cumulative", () => {
  it("running total", () => {
    expect(
      cumulative([
        { date: "a", total: 1 },
        { date: "b", total: 2 },
      ]).map((d) => d.total)
    ).toEqual([1, 3]);
  });
});
