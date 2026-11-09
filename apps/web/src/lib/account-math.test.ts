import { describe, expect, it } from "vitest";
import {
  accountFlows,
  buildAccountsOverview,
  daysUntil,
  fixedDepositProjection,
  monthlyInterestEstimate,
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
