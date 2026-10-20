import { z } from "zod";
import { formatCurrency } from "@/lib/format";
import type { ShareCounterpartyInput } from "@/types/api";
import { RECEIVABLE_KIND_LABELS } from "@/lib/receivable-format";
import type {
  ReceivableExpense,
  ReceivableKind,
  ShareMode,
  Transaction,
} from "@/types/models";

/** Max counterparties per shared expense (contract: 1..20). */
export const MAX_COUNTERPARTIES = 20;
/** Max counterparty name length in runes after normalization. */
const MAX_NAME_LENGTH = 100;

/**
 * User-facing "Money owed back" modes. A mode IS a receivable kind (loans are
 * created from the Owed to me page, not here); the share mode follows from it.
 */
export type OwedMode = Exclude<ReceivableKind, "loan">;

export const OWED_MODES: {
  value: OwedMode;
  label: string;
  hint: string;
  counterpartyLabel: string;
}[] = [
  {
    value: "split",
    label: "Split with others",
    hint: "You paid; others owe you their share.",
    counterpartyLabel: "Person",
  },
  {
    value: "on_behalf",
    label: "Paid for someone",
    hint: "You paid the full amount for someone else.",
    counterpartyLabel: "Person",
  },
  {
    value: "claim",
    label: "Work or insurance claim",
    hint: "An employer or insurer will pay you back.",
    counterpartyLabel: "Employer or insurer",
  },
  {
    value: "deposit",
    label: RECEIVABLE_KIND_LABELS.deposit,
    hint: "A deposit you expect to get back.",
    counterpartyLabel: "Held by",
  },
  {
    value: "refund",
    label: "Awaiting refund",
    hint: "A refund you are waiting for.",
    counterpartyLabel: "Refunded by",
  },
];

/**
 * Exactly Go's unicode.IsSpace set, which strings.Fields splits on:
 * \t \n \v \f \r, space, U+0085, U+00A0, U+1680, U+2000-U+200A, U+2028,
 * U+2029, U+202F, U+205F and U+3000. JS `\s` differs (it includes U+FEFF and
 * omits U+0085), so the set is spelled out here. Keep in sync with the server.
 */
const GO_SPACE = /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/;

/** Collapse whitespace exactly like Go's strings.Join(strings.Fields(raw), " "). */
export function normalizeCounterpartyName(raw: string): string {
  return raw.split(GO_SPACE).filter(Boolean).join(" ");
}

/**
 * Go's strings.ToLower maps each rune on its own (simple case mapping), unlike
 * JS toLowerCase which uses full, context-sensitive mappings (final sigma,
 * U+0130 becoming two code points). Lower-case code point by code point and keep
 * only the first resulting code point to match.
 */
function goToLower(s: string): string {
  let out = "";
  for (const ch of s) {
    const lower = ch.toLowerCase();
    out += [...lower].length === 1 ? lower : [...lower][0];
  }
  return out;
}

/** Lowercased normalized name, the dedupe key used by the server. */
export function counterpartyKey(raw: string): string {
  return goToLower(normalizeCounterpartyName(raw));
}

/** True when a name contains control characters (the server answers 400). */
function hasControlChars(name: string): boolean {
  return /\p{Cc}/u.test(name);
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
      return amount - n * equalShareAmount(amount, n);
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

const sharedExpenseSchema = z
  .object({
    amount: z.number().int().positive("Amount must be greater than zero"),
    mode: z.enum(["equal", "custom", "full"]),
    counterparties: z.array(
      z.object({
        name: z.string(),
        amount: z.number().optional(),
        /** An already-saved row (has an id). Only new rows are checked for duplicates. */
        existing: z.boolean().optional(),
      })
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

    // Mirrors the server's UpdateReceivables: saved rows may share a key (a
    // rename can merge counterparties), but a new row may not share a key with
    // any other row in the request.
    const keyCount = new Map<string, number>();
    for (const row of rows) {
      const k = counterpartyKey(row.name);
      keyCount.set(k, (keyCount.get(k) ?? 0) + 1);
    }
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
      if (hasControlChars(name)) {
        fail("Names can't contain control characters");
        return;
      }
      if (!row.existing && (keyCount.get(counterpartyKey(name)) ?? 0) > 1) {
        fail(`"${name}" is listed more than once`);
        return;
      }
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
  /** `id` marks an already-saved row (editor only); omit for new expenses. */
  rows: { name: string; amount: number; id?: string }[];
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
    counterparties: counterparties.map((c, i) => ({
      ...c,
      existing: Boolean(input.rows[i].id),
    })),
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

/** Stable client-side key for list rows. */
export function nextRowKey(): string {
  return crypto.randomUUID();
}

/** Trimmed expense view that receivable components take. */
export function toReceivableExpense(tx: Transaction): ReceivableExpense {
  return {
    id: tx.id,
    description: tx.description,
    date: tx.date,
    amount: tx.amount,
    personal_amount: tx.personal_amount ?? null,
    account_id: tx.account_id,
    category_id: tx.category_id ?? null,
  };
}
