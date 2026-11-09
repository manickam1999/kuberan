import { describe, expect, it } from "vitest";
import {
  EMPTY_TX_QUERY,
  activeFilterCount,
  dayNetByCurrency,
  groupByDay,
  majorToCents,
  parseTxQuery,
  periodRange,
  recentCategoryIds,
  sameCurrencySum,
  serializeTxQuery,
  toFilters,
} from "./transaction-list";
import type { Transaction } from "@/types/models";

function tx(partial: Partial<Transaction>): Transaction {
  return {
    id: "x",
    type: "expense",
    amount: 100,
    date: "2026-11-09T00:00:00Z",
    account_id: "a",
    ...partial,
  } as Transaction;
}

describe("groupByDay", () => {
  it("keeps order and groups by the date part", () => {
    const groups = groupByDay([
      tx({ id: "1", date: "2026-11-09T00:00:00Z" }),
      tx({ id: "2", date: "2026-11-09T23:00:00Z" }),
      tx({ id: "3", date: "2026-11-08T00:00:00Z" }),
    ]);
    expect(groups.map((g) => [g.key, g.items.length])).toEqual([
      ["2026-11-09", 2],
      ["2026-11-08", 1],
    ]);
  });
});

describe("dayNetByCurrency", () => {
  it("nets per currency and skips transfers", () => {
    const cur = (t: Transaction) => (t.account_id === "u" ? "USD" : "MYR");
    const nets = dayNetByCurrency(
      [
        tx({ type: "expense", amount: 500 }),
        tx({ type: "income", amount: 200 }),
        tx({ type: "expense", amount: 100, account_id: "u" }),
        tx({ type: "transfer", amount: 999 }),
      ],
      cur
    );
    expect(nets).toEqual([
      ["MYR", -300],
      ["USD", -100],
    ]);
  });

  it("drops currencies that net to zero", () => {
    const nets = dayNetByCurrency(
      [tx({ type: "expense", amount: 5 }), tx({ type: "income", amount: 5 })],
      () => "MYR"
    );
    expect(nets).toEqual([]);
  });
});

describe("sameCurrencySum", () => {
  it("sums one currency and refuses mixed or non-cash rows", () => {
    const cur = (t: Transaction) => (t.account_id === "u" ? "USD" : "MYR");
    expect(
      sameCurrencySum([tx({ amount: 300 }), tx({ type: "income", amount: 100 })], cur)
    ).toEqual({ amount: -200, currency: "MYR" });
    expect(sameCurrencySum([tx({}), tx({ account_id: "u" })], cur)).toBeNull();
    expect(sameCurrencySum([tx({ type: "transfer" })], cur)).toBeNull();
    expect(sameCurrencySum([], cur)).toBeNull();
  });
});

describe("periodRange", () => {
  const now = new Date(2026, 10, 9); // 9 Nov 2026
  it("resolves month periods", () => {
    expect(periodRange("this-month", now)).toEqual({
      from: "2026-11-01",
      to: "2026-11-30",
    });
    expect(periodRange("last-month", now)).toEqual({
      from: "2026-10-01",
      to: "2026-10-31",
    });
    expect(periodRange("last-90", now)).toEqual({
      from: "2026-08-12",
      to: "2026-11-09",
    });
  });
  it("is open for all and passes custom through", () => {
    expect(periodRange("all", now)).toEqual({});
    expect(periodRange("custom", now, { from: "2026-01-01", to: "" })).toEqual({
      from: "2026-01-01",
      to: undefined,
    });
  });
});

describe("query state", () => {
  it("round-trips non-default values", () => {
    const state = {
      ...EMPTY_TX_QUERY,
      q: "coffee",
      period: "last-month" as const,
      type: "expense",
      min: "10",
      max: "99.5",
      shared: true,
      page: 3,
    };
    const qs = serializeTxQuery(state);
    expect(parseTxQuery(new URLSearchParams(qs))).toEqual(state);
  });

  it("serialises defaults to nothing", () => {
    expect(serializeTxQuery(EMPTY_TX_QUERY)).toBe("");
  });

  it("ignores malformed params", () => {
    const parsed = parseTxQuery(
      new URLSearchParams("period=bogus&type=nope&min=abc&from=yesterday&page=-4")
    );
    expect(parsed).toEqual(EMPTY_TX_QUERY);
  });

  it("only keeps the custom dates for a custom period", () => {
    const qs = serializeTxQuery({
      ...EMPTY_TX_QUERY,
      period: "this-month",
      from: "2026-01-01",
    });
    expect(qs).toBe("period=this-month");
  });
});

describe("toFilters / activeFilterCount", () => {
  it("maps state to API filters in cents", () => {
    const f = toFilters(
      { ...EMPTY_TX_QUERY, min: "12.50", receipt: true, q: " cafe " },
      25
    );
    expect(f).toMatchObject({
      page: 1,
      page_size: 25,
      q: "cafe",
      min_amount: 1250,
      has_attachment: true,
    });
    expect(f.max_amount).toBeUndefined();
    expect(f.shared).toBeUndefined();
  });

  it("counts filters but not the search or page", () => {
    expect(
      activeFilterCount({ ...EMPTY_TX_QUERY, q: "x", page: 2 })
    ).toBe(0);
    expect(
      activeFilterCount({
        ...EMPTY_TX_QUERY,
        period: "last-90",
        account: "a",
        min: "1",
        shared: true,
      })
    ).toBe(4);
  });

  it("majorToCents rounds", () => {
    expect(majorToCents("0.29")).toBe(29);
    expect(majorToCents("")).toBeUndefined();
  });
});

describe("recentCategoryIds", () => {
  it("returns distinct newest-first ids of the type", () => {
    const rows = [
      { type: "expense" as const, category_id: "a", date: "2026-11-01T00:00:00Z" },
      { type: "expense" as const, category_id: "b", date: "2026-11-05T00:00:00Z" },
      { type: "income" as const, category_id: "i", date: "2026-11-06T00:00:00Z" },
      { type: "expense" as const, category_id: "a", date: "2026-11-07T00:00:00Z" },
      { type: "expense" as const, category_id: null, date: "2026-11-08T00:00:00Z" },
    ];
    expect(recentCategoryIds(rows, "expense", 5)).toEqual(["a", "b"]);
    expect(recentCategoryIds(rows, "expense", 1)).toEqual(["a"]);
  });
});
