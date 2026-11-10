import type { RulePrefill } from "@/lib/rule-prefill";
import type { RuleSuggestion } from "@/types/api";

const STORAGE_KEY = "kuberan.dismissed-rule-suggestions";

/** Identifies a suggestion for dismissal: the same pattern, category and type. */
export function suggestionKey(s: RuleSuggestion): string {
  return `${s.type}|${s.category_id}|${s.description_pattern}`;
}

/** The rule editor's starting point for a suggestion. */
export function suggestionPrefill(s: RuleSuggestion): RulePrefill {
  return {
    conditions: [
      {
        field: "description",
        operator: "contains",
        value_text: s.description_pattern.slice(0, 500),
      },
      { field: "type", operator: "equals", value_text: s.type },
    ],
    categoryId: s.category_id,
  };
}

/** The first suggestion the user has not dismissed, or null. */
export function firstVisibleSuggestion(
  suggestions: RuleSuggestion[] | undefined,
  dismissed: ReadonlySet<string>
): RuleSuggestion | null {
  return (
    (suggestions ?? []).find((s) => !dismissed.has(suggestionKey(s))) ?? null
  );
}

/** Dismissed suggestion keys remembered in this browser. */
export function loadDismissed(): Set<string> {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((v): v is string => typeof v === "string"));
    }
  } catch {
    // Storage blocked or corrupt: nothing is dismissed.
  }
  return new Set();
}

/** Remembers the dismissed keys (best effort; ignored when storage is blocked). */
export function saveDismissed(keys: ReadonlySet<string>): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify([...keys]));
  } catch {
    // Not persisted; the dismissal still applies for this visit.
  }
}

/** "matched 3 times · last Mar 3, 2026" (or "not matched yet"). */
export function matchSummary(
  count: number,
  lastMatchedAt: string | null,
  formatDate: (iso: string) => string
): string {
  if (count <= 0) return "Not matched yet";
  const times = count === 1 ? "once" : `${count} times`;
  return lastMatchedAt
    ? `Matched ${times} · last ${formatDate(lastMatchedAt)}`
    : `Matched ${times}`;
}
