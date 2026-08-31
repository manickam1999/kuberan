import type {
  RuleField,
  RuleOperator,
  TransactionRule,
  RuleCondition,
} from "@/types/models";
import type { RuleConditionInput } from "@/types/api";
import { formatCurrency } from "@/lib/format";

// Operators valid for each field, in display order. Mirrors the backend matrix.
export const OPERATORS_BY_FIELD: Record<RuleField, RuleOperator[]> = {
  description: ["contains", "not_contains", "equals", "starts_with", "ends_with"],
  amount: ["gt", "lt", "between"],
  account_id: ["equals"],
  type: ["equals"],
};

export const FIELD_LABELS: Record<RuleField, string> = {
  description: "Description",
  amount: "Amount",
  account_id: "Account",
  type: "Type",
};

export const OPERATOR_LABELS: Record<RuleOperator, string> = {
  contains: "contains",
  not_contains: "does not contain",
  equals: "is",
  starts_with: "starts with",
  ends_with: "ends with",
  gt: "greater than",
  lt: "less than",
  between: "between",
};

export const RULE_FIELDS: RuleField[] = [
  "description",
  "amount",
  "account_id",
  "type",
];

/** True when a condition is fully specified and safe to send/preview. */
export function isConditionComplete(c: RuleConditionInput): boolean {
  switch (c.field) {
    case "description":
    case "account_id":
    case "type":
      return !!c.value_text && c.value_text.trim() !== "";
    case "amount":
      if (c.operator === "gt") return c.amount_min != null && c.amount_min > 0;
      if (c.operator === "lt") return c.amount_max != null && c.amount_max > 0;
      if (c.operator === "between")
        return (
          c.amount_min != null &&
          c.amount_max != null &&
          c.amount_min > 0 &&
          c.amount_max >= c.amount_min
        );
      return false;
    default:
      return false;
  }
}

/**
 * Human-readable one-line summary of a condition, e.g.
 * "Description contains GRAB" or "Amount between RM10.00 and RM50.00".
 * accountName resolves an account_id condition's value to a label.
 */
export function summarizeCondition(
  c: RuleCondition,
  accountName?: (id: string) => string
): string {
  const field = FIELD_LABELS[c.field];
  const op = OPERATOR_LABELS[c.operator];
  if (c.field === "amount") {
    if (c.operator === "between") {
      return `${field} ${op} ${formatCurrency(c.amount_min ?? 0)} and ${formatCurrency(
        c.amount_max ?? 0
      )}`;
    }
    const value = c.operator === "gt" ? c.amount_min : c.amount_max;
    return `${field} ${op} ${formatCurrency(value ?? 0)}`;
  }
  if (c.field === "account_id") {
    const label = accountName ? accountName(c.value_text ?? "") : c.value_text;
    return `${field} ${op} ${label}`;
  }
  return `${field} ${op} ${c.value_text ?? ""}`.trim();
}

/** Summarize a rule's action target category name (v1: set_category). */
export function summarizeRuleTarget(rule: TransactionRule): string {
  const action = rule.actions?.[0];
  return action?.category?.name ?? "a category";
}
