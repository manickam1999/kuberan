import { describe, expect, it } from "vitest";
import { takesPlainActivity } from "./default-account";

describe("takesPlainActivity", () => {
  it("allows the spending account types", () => {
    for (const type of ["cash", "credit_card", "debt"] as const) {
      expect(takesPlainActivity({ type })).toBe(true);
    }
  });

  it("refuses accounts whose balance is derived or locked", () => {
    expect(takesPlainActivity({ type: "investment" })).toBe(false);
    expect(takesPlainActivity({ type: "fixed_deposit" })).toBe(false);
  });
});
