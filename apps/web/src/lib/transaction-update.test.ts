import { describe, expect, it } from "vitest";
import { toUpdateBody } from "./transaction-update";

describe("toUpdateBody", () => {
  it("sends an empty string to clear the category", () => {
    expect(toUpdateBody({ category_id: null })).toEqual({ category_id: "" });
    expect(toUpdateBody({ type: "income", category_id: null })).toEqual({
      type: "income",
      category_id: "",
    });
  });

  it("leaves a set or omitted category alone", () => {
    expect(toUpdateBody({ category_id: "abc" })).toEqual({ category_id: "abc" });
    expect(toUpdateBody({ amount: 5 })).toEqual({ amount: 5 });
    expect("category_id" in toUpdateBody({ amount: 5 })).toBe(false);
  });
});
