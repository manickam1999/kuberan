import type { Receivable, ReceivableKind } from "@/types/models";

export type OwedView = "open" | "settled" | "all";

export interface CurrencyAmount {
  currency: string;
  cents: number;
}

export interface PersonGroup {
  key: string;
  name: string;
  /** Receivables shown for the active view, oldest expense first. */
  items: Receivable[];
  /** Open outstanding per currency, never added across currencies. */
  outstanding: CurrencyAmount[];
  openCount: number;
  /** The oldest open item, when any is open. */
  oldestOpen: Receivable | null;
}

const isOpen = (r: Receivable) => r.outstanding > 0;

/** The date a receivable dates from: its expense, else when it was created. */
export function receivableDate(r: Receivable): string {
  return r.transaction?.date ?? r.created_at;
}

/**
 * Groups receivables by person for the active view. Open people come first,
 * the longest-owed on top; the rest follow by name.
 */
export function groupByPerson(
  receivables: readonly Receivable[],
  view: OwedView,
  kind?: ReceivableKind | null
): PersonGroup[] {
  const groups = new Map<string, PersonGroup & { latest: string }>();
  for (const r of receivables) {
    if (kind && r.kind !== kind) continue;
    const open = isOpen(r);
    if (view === "open" && !open) continue;
    if (view === "settled" && open) continue;

    const g = groups.get(r.counterparty_key) ?? {
      key: r.counterparty_key,
      name: r.counterparty_name,
      items: [],
      outstanding: [],
      openCount: 0,
      oldestOpen: null,
      latest: "",
    };
    // A renamed person: the most recently created item carries the current name.
    if (r.created_at > g.latest) {
      g.latest = r.created_at;
      g.name = r.counterparty_name;
    }
    g.items.push(r);
    if (open) {
      g.openCount += 1;
      const slot = g.outstanding.find((o) => o.currency === r.currency);
      if (slot) slot.cents += r.outstanding;
      else g.outstanding.push({ currency: r.currency, cents: r.outstanding });
      if (!g.oldestOpen || receivableDate(r) < receivableDate(g.oldestOpen)) {
        g.oldestOpen = r;
      }
    }
    groups.set(r.counterparty_key, g);
  }

  const list = [...groups.values()];
  for (const g of list) {
    g.items.sort((a, b) => receivableDate(a).localeCompare(receivableDate(b)));
  }
  return list
    .map((g) => {
      const { latest, ...group } = g;
      void latest;
      return group;
    })
    .sort((a, b) => {
      if (!!a.oldestOpen !== !!b.oldestOpen) return a.oldestOpen ? -1 : 1;
      if (a.oldestOpen && b.oldestOpen) {
        const d = receivableDate(a.oldestOpen).localeCompare(
          receivableDate(b.oldestOpen)
        );
        if (d !== 0) return d;
      }
      return a.name.localeCompare(b.name);
    });
}

/** Whole days between an ISO date and `now` (never negative). */
export function daysSince(iso: string, now: Date = new Date()): number {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return 0;
  return Math.max(0, Math.floor((now.getTime() - then.getTime()) / 86_400_000));
}

export interface OwedStats {
  openItems: number;
  people: number;
  /** Repaid so far per currency (paid_amount of every receivable). */
  repaid: CurrencyAmount[];
  settledItems: number;
  oldest: { receivable: Receivable; days: number } | null;
}

export function owedStats(
  receivables: readonly Receivable[],
  now: Date = new Date()
): OwedStats {
  const people = new Set<string>();
  const repaid: CurrencyAmount[] = [];
  let openItems = 0;
  let settledItems = 0;
  let oldest: Receivable | null = null;
  for (const r of receivables) {
    if (r.paid_amount > 0) {
      const slot = repaid.find((x) => x.currency === r.currency);
      if (slot) slot.cents += r.paid_amount;
      else repaid.push({ currency: r.currency, cents: r.paid_amount });
    }
    if (isOpen(r)) {
      openItems += 1;
      people.add(r.counterparty_key);
      if (!oldest || receivableDate(r) < receivableDate(oldest)) oldest = r;
    } else if (r.status === "settled") {
      settledItems += 1;
    }
  }
  return {
    openItems,
    people: people.size,
    repaid,
    settledItems,
    oldest: oldest
      ? { receivable: oldest, days: daysSince(receivableDate(oldest), now) }
      : null,
  };
}

/** Quick repayment amounts for an outstanding balance: full and (when it differs) half. */
export function quickAmounts(outstanding: number): {
  full: number;
  half: number | null;
} {
  const half = Math.floor(outstanding / 2);
  return { full: outstanding, half: half > 0 && half < outstanding ? half : null };
}

/** Stable 1 to 6 slot for an avatar colour (`--chart-N`), from the person's key. */
export function avatarSlot(key: string): number {
  let h = 0;
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return (h % 6) + 1;
}
