import type { Category } from "@/types/models";

export interface CategoryCard {
  category: Category;
  /** Direct subcategories, in name order. */
  children: Category[];
  /** This month's total in the base currency: the category plus its subcategories (cents, positive). */
  total: number;
}

/**
 * Builds the Categories grid: one card per top-level category (or per category
 * whose parent isn't in the list), with its subcategories' totals rolled up.
 * `totals` maps category id to cents in a single currency (the base currency),
 * so adding them is safe. Cards are sorted by total (largest first), then name.
 */
export function buildCategoryCards(
  categories: Category[],
  totals: ReadonlyMap<string, number>
): CategoryCard[] {
  const ids = new Set(categories.map((c) => c.id));
  const childrenOf = new Map<string, Category[]>();
  const top: Category[] = [];
  for (const c of categories) {
    if (c.parent_id && ids.has(c.parent_id)) {
      const list = childrenOf.get(c.parent_id) ?? [];
      list.push(c);
      childrenOf.set(c.parent_id, list);
    } else {
      top.push(c);
    }
  }
  const cards = top.map((category) => {
    const children = (childrenOf.get(category.id) ?? []).sort((a, b) =>
      a.name.localeCompare(b.name)
    );
    const total =
      (totals.get(category.id) ?? 0) +
      children.reduce((sum, c) => sum + (totals.get(c.id) ?? 0), 0);
    return { category, children, total };
  });
  return cards.sort(
    (a, b) => b.total - a.total || a.category.name.localeCompare(b.category.name)
  );
}
