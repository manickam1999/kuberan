import { describe, expect, it } from "vitest";
import { isBackgroundRefetch } from "./refresh-activity";

type Q = Parameters<typeof isBackgroundRefetch>[0];

function q(
  fetchStatus: "fetching" | "idle" | "paused",
  data: unknown,
  meta?: Record<string, unknown>
): Q {
  return { state: { fetchStatus, data }, meta } as unknown as Q;
}

describe("isBackgroundRefetch", () => {
  it("counts a refetch of a query that already has data", () => {
    expect(isBackgroundRefetch(q("fetching", { a: 1 }))).toBe(true);
  });

  it("ignores first loads (no data yet)", () => {
    expect(isBackgroundRefetch(q("fetching", undefined))).toBe(false);
  });

  it("ignores idle and paused (offline) queries", () => {
    expect(isBackgroundRefetch(q("idle", { a: 1 }))).toBe(false);
    expect(isBackgroundRefetch(q("paused", { a: 1 }))).toBe(false);
  });

  it("ignores polls marked silentRefetch", () => {
    expect(
      isBackgroundRefetch(q("fetching", { a: 1 }, { silentRefetch: true }))
    ).toBe(false);
  });

  it("counts falsy-but-present data such as an empty list", () => {
    expect(isBackgroundRefetch(q("fetching", []))).toBe(true);
    expect(isBackgroundRefetch(q("fetching", null))).toBe(true);
  });
});
