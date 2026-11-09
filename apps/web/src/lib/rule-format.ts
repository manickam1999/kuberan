import type {
  RuleField,
  RuleOperator,
  TransactionRule,
  RuleCondition,
} from "@/types/models";
import type { RuleConditionInput } from "@/types/api";
import { formatMoney } from "@/lib/format";

/**
 * Rules carry no currency of their own (an amount condition is compared with
 * the raw amount, whatever the account), so a bare number is shown unless the
 * caller knows one.
 */
function formatRuleAmount(cents: number, currency?: string): string {
  return currency
    ? formatMoney(cents, currency)
    : (cents / 100).toLocaleString("en-US", {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });
}

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
 * A condition split for sentence-style rendering: `lead` is the fixed wording
 * ("Description contains"), `value` the part to emphasise ("GRAB").
 */
export function conditionParts(
  c: RuleCondition,
  accountName?: (id: string) => string,
  currency?: string
): { lead: string; value: string } {
  const lead = `${FIELD_LABELS[c.field]} ${OPERATOR_LABELS[c.operator]}`;
  if (c.field === "amount") {
    if (c.operator === "between") {
      return {
        lead,
        value: `${formatRuleAmount(c.amount_min ?? 0, currency)} and ${formatRuleAmount(
          c.amount_max ?? 0,
          currency
        )}`,
      };
    }
    const amount = c.operator === "gt" ? c.amount_min : c.amount_max;
    return { lead, value: formatRuleAmount(amount ?? 0, currency) };
  }
  if (c.field === "account_id") {
    return {
      lead,
      value: accountName ? accountName(c.value_text ?? "") : (c.value_text ?? ""),
    };
  }
  return { lead, value: c.value_text ?? "" };
}

/**
 * Human-readable one-line summary of a condition, e.g.
 * "Description contains GRAB" or "Amount between RM10.00 and RM50.00".
 * accountName resolves an account_id condition's value to a label.
 */
export function summarizeCondition(
  c: RuleCondition,
  accountName?: (id: string) => string,
  currency?: string
): string {
  const field = FIELD_LABELS[c.field];
  const op = OPERATOR_LABELS[c.operator];
  if (c.field === "amount") {
    if (c.operator === "between") {
      return `${field} ${op} ${formatRuleAmount(c.amount_min ?? 0, currency)} and ${formatRuleAmount(
        c.amount_max ?? 0,
        currency
      )}`;
    }
    const value = c.operator === "gt" ? c.amount_min : c.amount_max;
    return `${field} ${op} ${formatRuleAmount(value ?? 0, currency)}`;
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

/**
 * Suggest a "description contains" value from a transaction description for the
 * "create rule from this transaction" flow. Picks the first alphabetic token of
 * length >= 3 (usually the merchant name, e.g. "GRAB" from "GRAB *RIDE 8823"),
 * falling back to the trimmed description. The user can edit it before saving.
 */
export function suggestConditionValue(description: string): string {
  const tokens = description.match(/[A-Za-z]{3,}/g);
  if (tokens && tokens.length > 0) return tokens[0];
  return description.trim();
}
