import { describe, expect, it } from "vitest";
import { parseRulePrefill } from "@/lib/rule-prefill";
import { conditionParts } from "@/lib/rule-format";
import type { RuleCondition } from "@/types/models";

const q = (s: string) => new URLSearchParams(s);

describe("parseRulePrefill", () => {
  it("needs new=1", () => {
    expect(parseRulePrefill(q("description=Shell"))).toBeNull();
    expect(parseRulePrefill(q(""))).toBeNull();
  });

  it("builds description and type conditions plus the category", () => {
    expect(
      parseRulePrefill(q("new=1&description=SHELL%20KLCC&category_id=abc&type=expense"))
    ).toEqual({
      conditions: [
        { field: "description", operator: "contains", value_text: "SHELL KLCC" },
        { field: "type", operator: "equals", value_text: "expense" },
      ],
      categoryId: "abc",
    });
  });

  it("ignores blank text and unknown types", () => {
    expect(parseRulePrefill(q("new=1&description=%20&type=transfer"))).toEqual({
      conditions: [],
      categoryId: undefined,
    });
  });
});

describe("conditionParts", () => {
  const base = { id: "1" } as RuleCondition;
  it("splits wording from value", () => {
    expect(
      conditionParts({ ...base, field: "description", operator: "contains", value_text: "GRAB" })
    ).toEqual({ lead: "Description contains", value: "GRAB" });
  });
  it("formats amount ranges", () => {
    const { value } = conditionParts(
      { ...base, field: "amount", operator: "between", amount_min: 1000, amount_max: 5000 },
      undefined,
      "MYR"
    );
    expect(value.replace(/\s/g, " ")).toBe("RM 10.00 and RM 50.00");
  });
});
