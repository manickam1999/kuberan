import { describe, expect, it } from "vitest";
import {
  firstVisibleSuggestion,
  matchSummary,
  suggestionKey,
  suggestionPrefill,
} from "@/lib/rule-suggestions";
import type { RuleSuggestion } from "@/types/api";

const a: RuleSuggestion = {
  description_pattern: "shell klcc",
  category_id: "c1",
  type: "expense",
  count: 4,
};
const b: RuleSuggestion = { ...a, description_pattern: "petron", count: 3 };

describe("suggestionPrefill", () => {
  it("builds a description and a type condition plus the category", () => {
    expect(suggestionPrefill(a)).toEqual({
      conditions: [
        { field: "description", operator: "contains", value_text: "shell klcc" },
        { field: "type", operator: "equals", value_text: "expense" },
      ],
      categoryId: "c1",
    });
  });
});

describe("firstVisibleSuggestion", () => {
  it("skips dismissed suggestions", () => {
    expect(firstVisibleSuggestion([a, b], new Set())).toBe(a);
    expect(firstVisibleSuggestion([a, b], new Set([suggestionKey(a)]))).toBe(b);
    expect(
      firstVisibleSuggestion([a], new Set([suggestionKey(a)]))
    ).toBeNull();
    expect(firstVisibleSuggestion(undefined, new Set())).toBeNull();
  });
});

describe("matchSummary", () => {
  const fmt = (iso: string) => `on ${iso.slice(0, 10)}`;
  it("describes counts and the last match", () => {
    expect(matchSummary(0, null, fmt)).toBe("Not matched yet");
    expect(matchSummary(1, "2026-03-03T10:00:00Z", fmt)).toBe(
      "Matched once · last on 2026-03-03"
    );
    expect(matchSummary(23, "2026-03-03T10:00:00Z", fmt)).toBe(
      "Matched 23 times · last on 2026-03-03"
    );
    expect(matchSummary(2, null, fmt)).toBe("Matched 2 times");
  });
});
