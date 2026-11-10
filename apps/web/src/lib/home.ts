import type { Account, PortfolioSnapshot, Transaction } from "@/types/models";

/** The three lines of the Home balance-sheet panel, in cents of the snapshot's currency. */
export interface BalanceSheet {
  /** Cash accounts plus fixed deposits. */
  cash: number;
  investments: number;
  /** Credit cards and loans, as a positive amount owed. */
  debt: number;
  /** Share (0-100) of the asset bar taken by cash; investments get the rest. */
  cashPct: number;
}

/**
 * Splits a portfolio snapshot into the balance-sheet lines. The bar compares
 * the two asset groups, so debt stays out of its proportions.
 */
export function balanceSheet(
  s: Pick<
    PortfolioSnapshot,
    "cash_balance" | "investment_value" | "debt_balance" | "fixed_deposit_balance"
  >
): BalanceSheet {
  const cash = s.cash_balance + (s.fixed_deposit_balance ?? 0);
  const investments = s.investment_value;
  const assets = cash + investments;
  const cashPct = assets > 0 ? Math.max(0, Math.min(100, (cash / assets) * 100)) : 0;
  return { cash, investments, debt: s.debt_balance, cashPct };
}

export interface OnboardingStep {
  id: "account" | "transaction" | "budget" | "assistant";
  title: string;
  description: string;
  done: boolean;
  /** Optional steps do not hold the user on the welcome screen. */
  optional: boolean;
}

export interface OnboardingInput {
  accounts: readonly Pick<Account, "id">[];
  /** Recent transactions, newest first; opening balances don't count. */
  transactions: readonly Pick<Transaction, "type">[];
  budgetCount: number;
  connectedAppCount: number;
}

/** True for a transaction the user entered, as opposed to an account's starting balance. */
export function isRealTransaction(t: Pick<Transaction, "type">): boolean {
  return t.type !== "opening_balance";
}

/** The four setup steps, computed from what the user already has. */
export function onboardingSteps(input: OnboardingInput): OnboardingStep[] {
  return [
    {
      id: "account",
      title: "Add your first account",
      description:
        "Cash, credit card, investment or fixed deposit. Opening balances don't count as income.",
      done: input.accounts.length > 0,
      optional: false,
    },
    {
      id: "transaction",
      title: "Add or import transactions",
      description:
        "Type them in, or send “coffee 6.50” to the Telegram bot once it is linked.",
      done: input.transactions.some(isRealTransaction),
      optional: false,
    },
    {
      id: "budget",
      title: "Set a budget or two",
      description:
        "Start with the categories you worry about. You can change them any time.",
      done: input.budgetCount > 0,
      optional: false,
    },
    {
      id: "assistant",
      title: "Connect your AI assistant",
      description:
        "Ask Claude, or any app that supports MCP, about your money. Optional.",
      done: input.connectedAppCount > 0,
      optional: true,
    },
  ];
}

/** Home shows the welcome checklist until there is an account with real activity. */
export function needsOnboarding(steps: readonly OnboardingStep[]): boolean {
  return !(steps[0]?.done && steps[1]?.done);
}

/** The local calendar day of a transaction date (`YYYY-MM-DD`, any time part ignored). */
function dayKey(iso: string): string {
  return iso.split("T")[0];
}

function localKey(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

/** "Today", "Yesterday", otherwise "Tue 27 Oct". */
export function dayLabel(iso: string, now: Date = new Date()): string {
  const key = dayKey(iso);
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  if (key === localKey(now)) return "Today";
  if (key === localKey(yesterday)) return "Yesterday";
  const [y, m, d] = key.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}

export interface DayGroup<T> {
  key: string;
  label: string;
  items: T[];
}

/** Groups transactions (already sorted) into consecutive days. */
export function groupByDay<T extends Pick<Transaction, "date">>(
  items: readonly T[],
  now: Date = new Date()
): DayGroup<T>[] {
  const groups: DayGroup<T>[] = [];
  for (const item of items) {
    const key = dayKey(item.date);
    const last = groups[groups.length - 1];
    if (last && last.key === key) {
      last.items.push(item);
    } else {
      groups.push({ key, label: dayLabel(item.date, now), items: [item] });
    }
  }
  return groups;
}

export type BudgetStatus = "over" | "close" | "ok";

/** Budget standing from its used percentage: over above 100, close from 75. */
export function budgetStatus(percentage: number): BudgetStatus {
  if (percentage > 100) return "over";
  if (percentage >= 75) return "close";
  return "ok";
}

/** Cumulative series of per-day totals, for the month spend line. */
export function cumulative(
  days: readonly { date: string; total: number }[]
): { date: string; total: number }[] {
  let sum = 0;
  return days.map((d) => {
    sum += d.total;
    return { date: d.date, total: sum };
  });
}

/** What the cash-flow card shows for one month. */
export interface MonthFlow {
  /** Income minus spending, in cents (negative when overspent). */
  net: number;
  /** True when spending exceeded income. */
  overspent: boolean;
  /**
   * Share of income kept, as a whole percent, only when something was saved
   * (a percentage of a tiny or zero income says nothing useful).
   */
  savedPercent: number | null;
}

/** Net flow for a month, for the cash-flow card's third figure. */
export function monthFlow(income: number, expenses: number): MonthFlow {
  const net = income - expenses;
  const overspent = net < 0;
  const savedPercent =
    !overspent && income > 0 ? Math.round((net / income) * 100) : null;
  return { net, overspent, savedPercent };
}
