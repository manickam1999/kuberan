import type { RuleConditionInput } from "@/types/api";

/** What `/rules?new=1&...` opens the rule editor with. */
export interface RulePrefill {
  conditions: RuleConditionInput[];
  categoryId?: string;
}

/**
 * Reads the "create rule from here" deep link:
 *
 *   /rules?new=1&description=<text>&category_id=<id>&type=<income|expense>
 *
 * Returns null unless `new=1`. `description` becomes a "description contains"
 * condition, `type` a "type is" condition (anything but income/expense is
 * ignored) and `category_id` the category to assign. All three are optional.
 */
export function parseRulePrefill(params: {
  get(name: string): string | null;
}): RulePrefill | null {
  if (params.get("new") !== "1") return null;
  const conditions: RuleConditionInput[] = [];
  const description = params.get("description")?.trim();
  if (description) {
    conditions.push({
      field: "description",
      operator: "contains",
      value_text: description.slice(0, 500),
    });
  }
  const type = params.get("type");
  if (type === "income" || type === "expense") {
    conditions.push({ field: "type", operator: "equals", value_text: type });
  }
  return {
    conditions,
    categoryId: params.get("category_id") || undefined,
  };
}
