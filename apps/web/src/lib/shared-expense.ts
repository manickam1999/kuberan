import { z } from "zod";
import { formatCurrency } from "@/lib/format";
import type { ShareCounterpartyInput } from "@/types/api";
import type { ReceivableKind, ShareMode } from "@/types/models";

/** Max counterparties per shared expense (contract: 1..20). */
export const MAX_COUNTERPARTIES = 20;
/** Max counterparty name length in runes after normalization. */
export const MAX_NAME_LENGTH = 100;

/** User-facing "Money owed back" modes. Each maps to a receivable kind + share mode. */
export type OwedMode = "split" | "on_behalf" | "claim" | "deposit" | "refund";

export const OWED_MODES: {
  value: OwedMode;
  label: string;
  kind: ReceivableKind;
  hint: string;
  counterpartyLabel: string;
}[] = [
  {
    value: "split",
    label: "Split with others",
    kind: "split",
    hint: "You paid; others owe you their share.",
    counterpartyLabel: "Person",
  },
  {
    value: "on_behalf",
    label: "Paid for someone",
    kind: "on_behalf",
    hint: "You paid the full amount for someone else.",
    counterpartyLabel: "Person",
  },
  {
    value: "claim",
    label: "Work or insurance claim",
    kind: "claim",
    hint: "An employer or insurer will pay you back.",
    counterpartyLabel: "Employer or insurer",
  },
  {
    value: "deposit",
    label: "Deposit",
    kind: "deposit",
    hint: "A deposit you expect to get back.",
    counterpartyLabel: "Held by",
  },
  {
    value: "refund",
    label: "Awaiting refund",
    kind: "refund",
    hint: "A refund you are waiting for.",
    counterpartyLabel: "Refunded by",
  },
];

/** Collapse whitespace exactly like Go's strings.Join(strings.Fields(raw), " "). */
export function normalizeCounterpartyName(raw: string): string {
  return raw.split(/\s+/).filter(Boolean).join(" ");
}

/** Lowercased normalized name, the dedupe key used by the server. */
export function counterpartyKey(raw: string): string {
  return normalizeCounterpartyName(raw).toLowerCase();
}

/** Resolve the wire `mode` for a UI selection. */
export function resolveShareMode(
  mode: OwedMode,
  splitMode: "equal" | "custom",
  partialClaim: boolean
): ShareMode {
  if (mode === "split") return splitMode;
  if (mode === "claim" && partialClaim) return "custom";
  return "full";
}

/**
 * What counts as the user's own spending (cents). Mirrors the server contract:
 * equal = amount - n * floor(amount / (n + 1)); custom = amount - sum; full = 0.
 */
export function computeYourShare(
  amount: number,
  mode: ShareMode,
  counterpartyAmounts: number[]
): number {
  const n = counterpartyAmounts.length;
  switch (mode) {
    case "equal":
      return amount - n * Math.floor(amount / (n + 1));
    case "custom":
      return amount - counterpartyAmounts.reduce((s, a) => s + a, 0);
    case "full":
      return 0;
  }
}

/** Per-counterparty amount owed in equal mode (cents). */
export function equalShareAmount(amount: number, n: number): number {
  return Math.floor(amount / (n + 1));
}

export const sharedExpenseSchema = z
  .object({
    amount: z.number().int().positive("Amount must be greater than zero"),
    mode: z.enum(["equal", "custom", "full"]),
    counterparties: z.array(
      z.object({ name: z.string(), amount: z.number().optional() })
    ),
  })
  .superRefine((val, ctx) => {
    const fail = (message: string) =>
      ctx.addIssue({ code: "custom", message, path: ["counterparties"] });
    const rows = val.counterparties;

    if (rows.length < 1 || rows.length > MAX_COUNTERPARTIES) {
      fail(`Add between 1 and ${MAX_COUNTERPARTIES} people`);
      return;
    }
    if (val.mode === "full" && rows.length !== 1) {
      fail("Exactly one person is required");
      return;
    }

    const seen = new Set<string>();
    for (const row of rows) {
      const name = normalizeCounterpartyName(row.name);
      const len = [...name].length;
      if (len < 1) {
        fail("Enter a name for everyone who owes you");
        return;
      }
      if (len > MAX_NAME_LENGTH) {
        fail(`Names can be at most ${MAX_NAME_LENGTH} characters`);
        return;
      }
      const key = name.toLowerCase();
      if (seen.has(key)) {
        fail(`"${name}" is listed more than once`);
        return;
      }
      seen.add(key);
    }

    if (val.mode === "custom") {
      let sum = 0;
      for (const row of rows) {
        if (!row.amount || row.amount <= 0) {
          fail("Enter an amount greater than zero for each person");
          return;
        }
        sum += row.amount;
      }
      if (sum > val.amount) {
        fail(
          `Amounts add up to ${formatCurrency(sum)}, more than the ${formatCurrency(val.amount)} expense`
        );
      }
    } else if (val.mode === "equal") {
      if (equalShareAmount(val.amount, rows.length) <= 0) {
        fail("Amount is too small to split between this many people");
      }
    }
  });

/**
 * Validate and normalize a shared-expense draft. Returns the counterparties as
 * the API expects them, or the first error message.
 */
export function buildSharedCounterparties(input: {
  amount: number;
  mode: ShareMode;
  rows: { name: string; amount: number }[];
}):
  | { ok: true; counterparties: ShareCounterpartyInput[]; yourShare: number }
  | { ok: false; error: string } {
  const counterparties: ShareCounterpartyInput[] = input.rows.map((r) => ({
    name: normalizeCounterpartyName(r.name),
    ...(input.mode === "custom" ? { amount: r.amount } : {}),
  }));
  const parsed = sharedExpenseSchema.safeParse({
    amount: input.amount,
    mode: input.mode,
    counterparties,
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "Invalid input" };
  }
  return {
    ok: true,
    counterparties,
    yourShare: computeYourShare(
      input.amount,
      input.mode,
      input.rows.map((r) => r.amount)
    ),
  };
}

let rowCounter = 0;
/** Stable client-side key for list rows. */
export function nextRowKey(): string {
  rowCounter += 1;
  return `row-${rowCounter}`;
}
