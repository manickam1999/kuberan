import { describe, expect, it } from "vitest";
import {
  avatarSlot,
  daysSince,
  groupByPerson,
  owedStats,
  quickAmounts,
} from "./owed";
import type { Receivable } from "@/types/models";

let n = 0;
function rec(over: Partial<Receivable>): Receivable {
  n += 1;
  return {
    id: `r${n}`,
    user_id: "u",
    transaction_id: `t${n}`,
    counterparty_name: "Arjun",
    counterparty_key: "arjun",
    kind: "loan",
    amount: 40000,
    paid_amount: 0,
    written_off_amount: 0,
    outstanding: 40000,
    status: "pending",
    currency: "MYR",
    created_at: `2026-10-0${(n % 9) + 1}T00:00:00Z`,
    updated_at: "2026-10-09T00:00:00Z",
    ...over,
  };
}

describe("groupByPerson", () => {
  const list = [
    rec({ counterparty_name: "Priya", counterparty_key: "priya", transaction: { id: "x", description: "Tickets", date: "2026-09-01T00:00:00Z", amount: 1, personal_amount: null, account_id: "a", category_id: null } }),
    rec({ counterparty_name: "Arjun", counterparty_key: "arjun", outstanding: 20000, paid_amount: 20000, status: "partially_paid", transaction: { id: "y", description: "Laptop", date: "2026-10-02T00:00:00Z", amount: 1, personal_amount: null, account_id: "a", category_id: null } }),
    rec({ counterparty_name: "Arjun", counterparty_key: "arjun", outstanding: 5000, currency: "USD", transaction: { id: "z", description: "Lunch", date: "2026-10-05T00:00:00Z", amount: 1, personal_amount: null, account_id: "a", category_id: null } }),
    rec({ counterparty_name: "Daniel", counterparty_key: "daniel", outstanding: 0, paid_amount: 8500, status: "settled", kind: "split" }),
  ];

  it("keeps open items, longest-owed person first, sums per currency", () => {
    const g = groupByPerson(list, "open");
    expect(g.map((x) => x.key)).toEqual(["priya", "arjun"]);
    const arjun = g[1];
    expect(arjun.items).toHaveLength(2);
    expect(arjun.outstanding).toEqual([
      { currency: "MYR", cents: 20000 },
      { currency: "USD", cents: 5000 },
    ]);
    expect(arjun.oldestOpen?.transaction?.description).toBe("Laptop");
  });

  it("settled view lists only closed items", () => {
    const g = groupByPerson(list, "settled");
    expect(g.map((x) => x.key)).toEqual(["daniel"]);
    expect(g[0].outstanding).toEqual([]);
  });

  it("filters by kind and includes everyone in the all view", () => {
    expect(groupByPerson(list, "all", "split").map((x) => x.key)).toEqual(["daniel"]);
    expect(groupByPerson(list, "all")).toHaveLength(3);
  });
});

describe("owedStats", () => {
  it("counts people, repaid per currency and the oldest open item", () => {
    const list = [
      rec({ counterparty_key: "a", paid_amount: 100, outstanding: 50, created_at: "2026-10-01T00:00:00Z" }),
      rec({ counterparty_key: "b", paid_amount: 300, outstanding: 0, status: "settled" }),
      rec({ counterparty_key: "a", outstanding: 10, created_at: "2026-09-01T00:00:00Z" }),
    ];
    const s = owedStats(list, new Date("2026-10-31T00:00:00Z"));
    expect(s.openItems).toBe(2);
    expect(s.people).toBe(1);
    expect(s.settledItems).toBe(1);
    expect(s.repaid).toEqual([{ currency: "MYR", cents: 400 }]);
    expect(s.oldest?.days).toBe(60);
  });
});

describe("quickAmounts", () => {
  it("offers half only when it is a distinct, positive amount", () => {
    expect(quickAmounts(40000)).toEqual({ full: 40000, half: 20000 });
    expect(quickAmounts(1)).toEqual({ full: 1, half: null });
    expect(quickAmounts(101)).toEqual({ full: 101, half: 50 });
  });
});

describe("daysSince / avatarSlot", () => {
  it("never goes negative", () => {
    expect(daysSince("2030-01-01T00:00:00Z", new Date("2026-01-01T00:00:00Z"))).toBe(0);
  });
  it("is stable and within 1..6", () => {
    expect(avatarSlot("arjun")).toBe(avatarSlot("arjun"));
    for (const k of ["a", "priya", "wei ling", ""]) {
      expect(avatarSlot(k)).toBeGreaterThanOrEqual(1);
      expect(avatarSlot(k)).toBeLessThanOrEqual(6);
    }
  });
});
