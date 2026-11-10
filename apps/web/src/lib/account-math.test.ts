import { describe, expect, it } from "vitest";
import {
  accountFlows,
  buildAccountsOverview,
  daysUntil,
  fixedDepositProjection,
  dueDayOf,
  formatDuration,
  loanPayoff,
  monthlyInterestEstimate,
  nextDueByDay,
  nextDueDate,
  utilisation,
} from "./account-math";
import type { Account, Transaction } from "@/types/models";

function acct(over: Partial<Account>): Account {
  return {
    id: "a",
    user_id: "u",
    name: "A",
    type: "cash",
    description: "",
    balance: 0,
    currency: "MYR",
    is_active: true,
    is_pinned: false,
    ...over,
  } as Account;
}

describe("buildAccountsOverview", () => {
  it("sums base balances only and reports accounts without a rate", () => {
    const o = buildAccountsOverview(
      [
        acct({ id: "1", balance: 100_00 }),
        acct({
          id: "2",
          balance: 50_00,
          currency: "USD",
          balance_base: 230_00,
        }),
        acct({ id: "3", balance: 70_00, currency: "EUR", fx_missing: true }),
        acct({ id: "4", type: "credit_card", balance: 40_00 }),
      ],
      "MYR",
      false,
    );
    expect(o.assets).toBe(330_00);
    expect(o.liabilities).toBe(40_00);
    expect(o.netWorth).toBe(290_00);
    expect(o.unconverted).toEqual([{ currency: "EUR", amount: 70_00 }]);
    const cash = o.groups.find((g) => g.key === "cash")!;
    expect(cash.totalBase).toBe(330_00);
    expect(o.groups.find((g) => g.key === "liability")!.totalBase).toBe(-40_00);
  });

  it("keeps inactive accounts out of totals and out of view unless shown", () => {
    const list = [
      acct({ id: "1", balance: 10_00 }),
      acct({ id: "2", balance: 99_00, is_active: false }),
    ];
    const hidden = buildAccountsOverview(list, "MYR", false);
    expect(hidden.groups[0].accounts).toHaveLength(1);
    expect(hidden.inactiveCount).toBe(1);
    const shown = buildAccountsOverview(list, "MYR", true);
    expect(shown.groups[0].accounts).toHaveLength(2);
    expect(shown.assets).toBe(10_00);
    expect(shown.groups[0].totalBase).toBe(10_00);
  });
});

describe("accountFlows", () => {
  const tx = (o: Partial<Transaction>) =>
    ({ id: "t", account_id: "a", amount: 0, ...o }) as Transaction;
  it("counts transfers by direction and ignores opening balances", () => {
    const f = accountFlows(
      [
        tx({ type: "income", amount: 500 }),
        tx({ type: "expense", amount: 120 }),
        tx({ type: "opening_balance", amount: 9000 }),
        tx({ type: "transfer", amount: 300, to_account_id: "b" }),
        tx({
          type: "transfer",
          amount: 400,
          to_amount: 90,
          to_account_id: "a",
        }),
        tx({ type: "reimbursement", amount: 50 }),
      ],
      "a",
    );
    expect(f).toEqual({ in: 640, out: 420, count: 6 });
  });
});

describe("credit card math", () => {
  it("computes utilisation and one month of interest", () => {
    expect(utilisation(6717_86, 15000_00)).toBeCloseTo(44.79, 1);
    expect(utilisation(10, null)).toBeNull();
    expect(monthlyInterestEstimate(6717_86, 21.9)).toBe(12260);
    expect(monthlyInterestEstimate(0, 21.9)).toBe(0);
    expect(monthlyInterestEstimate(100_00, 0)).toBe(0);
  });

  it("rolls the due day forward and clamps to short months", () => {
    const today = new Date(2026, 10, 9); // 9 Nov 2026
    expect(nextDueDate("2026-08-12T00:00:00Z", today)).toBe(
      Date.UTC(2026, 10, 12),
    );
    expect(nextDueDate("2026-11-09", today)).toBe(Date.UTC(2026, 10, 9));
    expect(nextDueDate("2026-01-05", today)).toBe(Date.UTC(2026, 11, 5));
    expect(nextDueDate("2026-01-31", new Date(2026, 1, 10))).toBe(
      Date.UTC(2026, 1, 28),
    );
    expect(nextDueDate(undefined, today)).toBeNull();
    expect(daysUntil(Date.UTC(2026, 10, 12), today)).toBe(3);
  });
});

describe("fixedDepositProjection", () => {
  const fd = {
    principal: 20000_00,
    start_date: "2026-01-01T00:00:00Z",
    maturity_date: "2027-01-01T00:00:00Z",
    compounding: "monthly" as const,
    payout_type: "cumulative" as const,
    maturity_value: 0,
  };

  it("compounds with the server's actual/365 formula", () => {
    const p = fixedDepositProjection(fd, 4.1, new Date(2027, 5, 1));
    const expected =
      Math.floor(20000_00 * Math.pow(1 + 0.041 / 12, 12) + 0.5) - 20000_00;
    expect(p.projected).toBe(expected);
    expect(p.progress).toBe(100);
    expect(p.accrued).toBe(p.projected);
    expect(p.maturityValue).toBe(20000_00 + p.projected);
  });

  it("accrues part-way through the term and prefers the server maturity value", () => {
    const p = fixedDepositProjection(
      { ...fd, maturity_value: 20835_70 },
      4.1,
      new Date(2026, 6, 2),
    );
    expect(p.totalDays).toBe(365);
    expect(p.elapsedDays).toBe(182);
    expect(p.remainingDays).toBe(183);
    expect(p.projected).toBe(835_70);
    expect(p.accrued).toBeGreaterThan(0);
    expect(p.accrued).toBeLessThan(p.projected);
  });

  it("uses simple interest for periodic payouts and is zero before the start", () => {
    const periodic = { ...fd, payout_type: "periodic" as const };
    const p = fixedDepositProjection(periodic, 3.65, new Date(2026, 0, 11));
    // 20000.00 * 3.65% * 10 / 365 = 20.00
    expect(p.accrued).toBe(20_00);
    expect(fixedDepositProjection(fd, 4.1, new Date(2025, 5, 1)).accrued).toBe(
      0,
    );
    expect(fixedDepositProjection(fd, 0, new Date(2026, 5, 1)).projected).toBe(
      0,
    );
  });
});

describe("nextDueByDay", () => {
  const today = new Date(2026, 10, 9); // 9 Nov 2026

  it("is this month when the day is still ahead, or today", () => {
    expect(nextDueByDay(20, today)).toBe(Date.UTC(2026, 10, 20));
    expect(nextDueByDay(9, today)).toBe(Date.UTC(2026, 10, 9));
  });

  it("rolls to next month once the day has passed, across the year end", () => {
    expect(nextDueByDay(1, today)).toBe(Date.UTC(2026, 11, 1));
    expect(nextDueByDay(5, new Date(2026, 11, 20))).toBe(Date.UTC(2027, 0, 5));
  });

  it("clamps to short months", () => {
    expect(nextDueByDay(31, today)).toBe(Date.UTC(2026, 10, 30));
    expect(nextDueByDay(31, new Date(2027, 1, 10))).toBe(Date.UTC(2027, 1, 28));
  });
});

describe("dueDayOf", () => {
  it("prefers due_day, falls back to the day of a legacy due_date", () => {
    expect(dueDayOf({ due_day: 15, due_date: "2026-01-03T00:00:00Z" })).toBe(15);
    expect(dueDayOf({ due_date: "2026-01-03T00:00:00Z" })).toBe(3);
    expect(dueDayOf({})).toBeNull();
    // An unset date arrives as the zero time.
    expect(dueDayOf({ due_date: "0001-01-01T00:00:00Z" })).toBeNull();
  });
});

describe("loanPayoff", () => {
  const today = new Date(2026, 10, 9);

  it("pays off an interest-free loan in whole payments, the last one smaller", () => {
    // 1,000.00 at 300.00 a month: 300 + 300 + 300 + 100.
    expect(loanPayoff(100_000, 0, 30_000, 15, today)).toEqual({
      months: 4,
      totalInterest: 0,
      finalPayment: 10_000,
      // First payment 15 Nov, fourth on 15 Feb 2027.
      payoffDate: Date.UTC(2027, 1, 15),
    });
  });

  it("adds interest of APR / 12 on the remaining balance each month", () => {
    // 12% APR is 1% a month. 1,000.00: +10.00, -300.00 -> 710.00; +7.10 -> 417.10;
    // +4.17 -> 121.27; +1.21 -> final payment 122.48.
    expect(loanPayoff(100_000, 12, 30_000, 1, today)).toEqual({
      months: 4,
      totalInterest: 1000 + 710 + 417 + 121,
      finalPayment: 12_248,
      payoffDate: Date.UTC(2027, 2, 1),
    });
  });

  it("is null when the payment does not cover the interest", () => {
    // 1% a month on 100,000.00 is 1,000.00 of interest.
    expect(loanPayoff(10_000_000, 12, 50_000, 1, today)).toBeNull();
    expect(loanPayoff(10_000_000, 12, 100_000, 1, today)).toBeNull();
  });

  it("is null without a balance or a payment", () => {
    expect(loanPayoff(0, 5, 10_000, 1, today)).toBeNull();
    expect(loanPayoff(10_000, 5, 0, 1, today)).toBeNull();
    expect(loanPayoff(10_000, 5, undefined, 1, today)).toBeNull();
  });

  it("clamps the payoff day to short months", () => {
    const p = loanPayoff(20_000, 0, 10_000, 31, new Date(2027, 0, 10));
    expect(p?.months).toBe(2);
    expect(p?.payoffDate).toBe(Date.UTC(2027, 1, 28));
  });

  it("uses today's day when no due day is set", () => {
    expect(loanPayoff(20_000, 0, 10_000, null, today)?.payoffDate).toBe(
      Date.UTC(2026, 11, 9),
    );
  });

  it("clears a balance smaller than one instalment in a single payment", () => {
    const p = loanPayoff(5_000, 0, 10_000, 20, today);
    expect(p).toMatchObject({ months: 1, finalPayment: 5_000 });
    expect(p?.payoffDate).toBe(Date.UTC(2026, 10, 20));
  });
});

describe("formatDuration", () => {
  it("words years and months", () => {
    expect(formatDuration(1)).toBe("1 month");
    expect(formatDuration(7)).toBe("7 months");
    expect(formatDuration(12)).toBe("1 year");
    expect(formatDuration(38)).toBe("3 years 2 months");
  });
});
