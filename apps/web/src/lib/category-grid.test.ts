import { describe, expect, it } from "vitest";
import { buildCategoryCards } from "@/lib/category-grid";
import type { Category } from "@/types/models";

function cat(id: string, name: string, parent_id?: string): Category {
  return { id, name, parent_id, type: "expense" } as Category;
}

describe("buildCategoryCards", () => {
  const cats = [
    cat("a", "Dining"),
    cat("b", "Coffee", "a"),
    cat("c", "Hawker", "a"),
    cat("d", "Travel"),
    cat("e", "Orphan", "gone"),
  ];

  it("rolls subcategory totals into the parent and sorts by total", () => {
    const cards = buildCategoryCards(
      cats,
      new Map([
        ["a", 1000],
        ["b", 250],
        ["c", 50],
        ["d", 5000],
      ])
    );
    expect(cards.map((c) => [c.category.name, c.total])).toEqual([
      ["Travel", 5000],
      ["Dining", 1300],
      ["Orphan", 0],
    ]);
    expect(cards[1].children.map((c) => c.name)).toEqual(["Coffee", "Hawker"]);
  });

  it("breaks ties by name", () => {
    const cards = buildCategoryCards([cat("x", "B"), cat("y", "A")], new Map());
    expect(cards.map((c) => c.category.name)).toEqual(["A", "B"]);
  });
});
