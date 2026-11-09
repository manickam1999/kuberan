import type {
  Account,
  AccountType,
  Compounding,
  FixedDepositDetails,
  Transaction,
} from "@/types/models";

/**
 * Pure helpers behind the account screens: balance-sheet grouping, credit card
 * utilisation and interest, and fixed deposit accrual. All money is int64 cents.
 * Day counts follow the server (`internal/services/fdmath`): actual/365 on UTC
 * calendar dates, half-up rounding to the cent.
 */

const DAY_MS = 86_400_000;

/** `YYYY-MM-DD` of an ISO string, as a UTC midnight timestamp. */
function utcDay(iso: string): number {
  const [y, m, d] = iso.slice(0, 10).split("-").map(Number);
  return Date.UTC(y, m - 1, d);
}

/** The local calendar day of a Date, as a UTC midnight timestamp. */
function localDayAsUtc(date: Date): number {
  return Date.UTC(date.getFullYear(), date.getMonth(), date.getDate());
}

function roundCents(v: number): number {
  return Math.floor(v + 0.5);
}

// ---- Balance-sheet grouping ----

export type AccountGroupKey =
  | "cash"
  | "fixed_deposit"
  | "investment"
  | "liability";

export const ACCOUNT_GROUP_LABELS: Record<AccountGroupKey, string> = {
  cash: "Cash & savings",
  fixed_deposit: "Fixed deposits",
  investment: "Investments",
  liability: "Credit cards & loans",
};

const GROUP_ORDER: AccountGroupKey[] = [
  "cash",
  "fixed_deposit",
  "investment",
  "liability",
];

export function accountGroupKey(type: AccountType): AccountGroupKey {
  switch (type) {
    case "cash":
      return "cash";
    case "fixed_deposit":
      return "fixed_deposit";
    case "investment":
      return "investment";
    default:
      return "liability";
  }
}

export function isLiability(account: Pick<Account, "type">): boolean {
  return account.type === "credit_card" || account.type === "debt";
}

/** The account's balance in the base currency, or null when no rate exists. */
export function baseBalance(
  account: Account,
  baseCurrency: string,
): number | null {
  if (account.balance_base !== undefined && account.balance_base !== null) {
    return account.balance_base;
  }
  return account.currency === baseCurrency ? account.balance : null;
}

/**
 * The balance with its balance-sheet sign: what a liability owes is negative.
 * (Credit card and debt balances are stored as the amount owed.)
 */
export function signedBalance(account: Account): number {
  return isLiability(account) ? -account.balance : account.balance;
}

export interface Unconverted {
  currency: string;
  amount: number;
}

export interface AccountGroup {
  key: AccountGroupKey;
  label: string;
  accounts: Account[];
  /** Signed total in the base currency (liabilities negative); excludes accounts without a rate. */
  totalBase: number;
  /** Active accounts that contributed to `totalBase` (had a base value). */
  convertedCount: number;
  unconverted: Unconverted[];
}

export interface AccountsOverview {
  groups: AccountGroup[];
  /** Sums over active accounts only, base currency. Liabilities are a positive amount owed. */
  assets: number;
  liabilities: number;
  netWorth: number;
  /** Active accounts with a base value; 0 means every total is just "no rate". */
  convertedCount: number;
  unconverted: Unconverted[];
  stale: boolean;
  inactiveCount: number;
}

function addUnconverted(map: Map<string, number>, account: Account) {
  map.set(
    account.currency,
    (map.get(account.currency) ?? 0) + signedBalance(account),
  );
}

function toList(map: Map<string, number>): Unconverted[] {
  return Array.from(map, ([currency, amount]) => ({ currency, amount }));
}

/**
 * Group accounts like a balance sheet. Totals add `balance_base` only (never
 * raw balances across currencies) and cover active accounts; inactive ones are
 * listed, without counting, when `showInactive` is set.
 */
export function buildAccountsOverview(
  accounts: Account[],
  baseCurrency: string,
  showInactive: boolean,
): AccountsOverview {
  const active = accounts.filter((a) => a.is_active);
  const visible = showInactive ? accounts : active;

  const groups: AccountGroup[] = GROUP_ORDER.map((key) => {
    const list = visible
      .filter((a) => accountGroupKey(a.type) === key)
      .sort(
        (a, b) =>
          Number(b.is_active) - Number(a.is_active) ||
          Number(b.is_pinned) - Number(a.is_pinned) ||
          Math.abs(baseBalance(b, baseCurrency) ?? 0) -
            Math.abs(baseBalance(a, baseCurrency) ?? 0) ||
          a.name.localeCompare(b.name),
      );
    let total = 0;
    let converted = 0;
    const missing = new Map<string, number>();
    for (const a of list) {
      if (!a.is_active) continue;
      const base = baseBalance(a, baseCurrency);
      if (base === null) addUnconverted(missing, a);
      else {
        total += isLiability(a) ? -base : base;
        converted += 1;
      }
    }
    return {
      key,
      label: ACCOUNT_GROUP_LABELS[key],
      accounts: list,
      totalBase: total,
      convertedCount: converted,
      unconverted: toList(missing),
    };
  }).filter((g) => g.accounts.length > 0);

  let assets = 0;
  let liabilities = 0;
  let convertedCount = 0;
  const missing = new Map<string, number>();
  for (const a of active) {
    const base = baseBalance(a, baseCurrency);
    if (base === null) {
      addUnconverted(missing, a);
      continue;
    }
    convertedCount += 1;
    if (isLiability(a)) liabilities += base;
    else assets += base;
  }

  return {
    groups,
    assets,
    liabilities,
    netWorth: assets - liabilities,
    convertedCount,
    unconverted: toList(missing),
    stale: active.some((a) => a.fx_stale && a.currency !== baseCurrency),
    inactiveCount: accounts.length - active.length,
  };
}

// ---- Cash account flows ----

export interface MonthFlows {
  in: number;
  out: number;
  count: number;
}

/**
 * Money in and out of one account over a set of its transactions, in the
 * account's own currency. Opening balances are neither: they don't count as
 * income. A transfer in counts what the account received (`to_amount`).
 */
export function accountFlows(
  transactions: Transaction[],
  accountId: string,
): MonthFlows {
  let inflow = 0;
  let outflow = 0;
  for (const t of transactions) {
    if (t.type === "opening_balance") continue;
    if (t.type === "transfer") {
      if (t.to_account_id === accountId) inflow += t.to_amount ?? t.amount;
      else outflow += t.amount;
    } else if (t.type === "income" || t.type === "reimbursement") {
      inflow += t.amount;
    } else {
      outflow += t.amount;
    }
  }
  return { in: inflow, out: outflow, count: transactions.length };
}

/** Whether a transaction is money out of the account being viewed. */
export function isOutflow(t: Transaction, accountId: string): boolean {
  if (t.type === "transfer") return t.to_account_id !== accountId;
  return t.type === "expense" || t.type === "investment";
}

// ---- Credit cards ----

/** Share of the limit in use, 0 to 100+; null without a limit. */
export function utilisation(
  owed: number,
  limit?: number | null,
): number | null {
  if (!limit || limit <= 0) return null;
  return (Math.max(owed, 0) / limit) * 100;
}

/** Roughly one month of interest on a carried balance (APR / 12). */
export function monthlyInterestEstimate(
  owed: number,
  aprPercent?: number | null,
): number {
  if (owed <= 0 || !aprPercent || aprPercent <= 0) return 0;
  return roundCents((owed * aprPercent) / 100 / 12);
}

/**
 * The next occurrence of a recurring due day. The stored due date is one
 * calendar date; its day of the month repeats (clamped to short months).
 * Returns a UTC-midnight timestamp, or null without a due date.
 */
export function nextDueDate(
  dueISO: string | undefined | null,
  today: Date,
): number | null {
  if (!dueISO) return null;
  return nextDueByDay(new Date(utcDay(dueISO)).getUTCDate(), today);
}

/**
 * The next occurrence of a due day of the month (1-31), clamped to short
 * months. Returns a UTC-midnight timestamp.
 */
export function nextDueByDay(dueDay: number, today: Date): number {
  const now = localDayAsUtc(today);
  const nowDate = new Date(now);
  let y = nowDate.getUTCFullYear();
  let m = nowDate.getUTCMonth();
  for (let i = 0; i < 2; i++) {
    const lastDay = new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
    const candidate = Date.UTC(y, m, Math.min(dueDay, lastDay));
    if (candidate >= now) return candidate;
    m += 1;
    if (m > 11) {
      m = 0;
      y += 1;
    }
  }
  return now;
}

/**
 * The day of the month a card or loan falls due: the `due_day` set on the
 * account, else the day of a legacy single `due_date`. Null when neither.
 */
export function dueDayOf(
  account: Pick<Account, "due_day" | "due_date">,
): number | null {
  if (account.due_day) return account.due_day;
  // The API sends an unset due_date as the zero time (0001-01-01).
  if (account.due_date && !account.due_date.startsWith("0001-")) {
    return new Date(utcDay(account.due_date)).getUTCDate();
  }
  return null;
}

/** Whole days from `today` to a UTC-midnight timestamp. */
export function daysUntil(target: number, today: Date): number {
  return Math.round((target - localDayAsUtc(today)) / DAY_MS);
}

// ---- Fixed deposits ----

const COMPOUNDING_PERIODS: Record<Compounding, number> = {
  monthly: 12,
  quarterly: 4,
  semiannual: 2,
  annual: 1,
};

export interface FixedDepositProjection {
  totalDays: number;
  elapsedDays: number;
  remainingDays: number;
  /** 0 to 100 through the term. */
  progress: number;
  /** Interest earned so far (compounded for cumulative deposits, simple otherwise). */
  accrued: number;
  /** Interest over the whole term. */
  projected: number;
  /** Principal plus projected interest. */
  maturityValue: number;
}

type FdInput = Pick<
  FixedDepositDetails,
  | "principal"
  | "start_date"
  | "maturity_date"
  | "compounding"
  | "payout_type"
  | "maturity_value"
>;

function compoundedInterest(
  principal: number,
  ratePercent: number,
  periodsPerYear: number,
  days: number,
): number {
  if (principal <= 0 || ratePercent <= 0 || days <= 0) return 0;
  const r = ratePercent / 100;
  const grown =
    principal * Math.pow(1 + r / periodsPerYear, (periodsPerYear * days) / 365);
  return roundCents(grown) - principal;
}

function simpleInterest(
  principal: number,
  ratePercent: number,
  days: number,
): number {
  if (principal <= 0 || ratePercent <= 0 || days <= 0) return 0;
  return roundCents((principal * (ratePercent / 100) * days) / 365);
}

/**
 * Accrued and projected interest of a fixed deposit as of a date, from its
 * rate, compounding and dates. Cumulative deposits compound
 * (`P (1 + r/n)^(n t) - P`, the server's maturity formula); periodic ones pay
 * simple interest out, so accrual is pro-rata simple. The server's own
 * `maturity_value` wins for the projection when present.
 */
export function fixedDepositProjection(
  fd: FdInput,
  ratePercent: number,
  asOf: Date,
): FixedDepositProjection {
  const start = utcDay(fd.start_date);
  const end = utcDay(fd.maturity_date);
  const now = localDayAsUtc(asOf);
  const totalDays = Math.max(0, Math.round((end - start) / DAY_MS));
  const elapsedDays = Math.min(
    Math.max(Math.round((now - start) / DAY_MS), 0),
    totalDays,
  );
  const periodic = fd.payout_type === "periodic";
  const n = COMPOUNDING_PERIODS[fd.compounding] ?? 1;

  const interestFor = (days: number) =>
    periodic
      ? simpleInterest(fd.principal, ratePercent, days)
      : compoundedInterest(fd.principal, ratePercent, n, days);

  const projected =
    !periodic && fd.maturity_value > 0
      ? Math.max(fd.maturity_value - fd.principal, 0)
      : interestFor(totalDays);

  return {
    totalDays,
    elapsedDays,
    remainingDays: totalDays - elapsedDays,
    progress: totalDays > 0 ? (elapsedDays / totalDays) * 100 : 0,
    accrued: Math.min(interestFor(elapsedDays), projected),
    projected,
    maturityValue: fd.principal + projected,
  };
}

// ---- Loans ----

/** Longest payoff worth projecting; beyond it the loan "never" clears. */
const MAX_PAYOFF_MONTHS = 1200;

export interface LoanPayoff {
  /** Payments left, the last one possibly smaller. */
  months: number;
  /** Interest paid over those payments, in cents. */
  totalInterest: number;
  /** The size of the final (partial) payment, in cents. */
  finalPayment: number;
  /** UTC-midnight timestamp of the last payment. */
  payoffDate: number;
}

/**
 * When a loan is paid off at its monthly payment: month by month, interest of
 * APR / 12 on the balance is added and the payment taken off. First payment on
 * the next due day (today when no due day is set). Returns null when there is
 * nothing to project (no balance or payment) or when the payment does not
 * cover the interest, so the balance never falls. An estimate: no fees, a
 * fixed rate, payments always on time.
 */
export function loanPayoff(
  owed: number,
  aprPercent: number | null | undefined,
  monthlyPayment: number | null | undefined,
  dueDay: number | null,
  today: Date,
): LoanPayoff | null {
  if (owed <= 0 || !monthlyPayment || monthlyPayment <= 0) return null;
  const rate = aprPercent && aprPercent > 0 ? aprPercent : 0;
  if (rate > 0 && monthlyInterestEstimate(owed, rate) >= monthlyPayment) {
    return null;
  }

  let balance = owed;
  let totalInterest = 0;
  let months = 0;
  let finalPayment = 0;
  while (balance > 0 && months < MAX_PAYOFF_MONTHS) {
    const interest = monthlyInterestEstimate(balance, rate);
    totalInterest += interest;
    balance += interest;
    months += 1;
    finalPayment = Math.min(monthlyPayment, balance);
    balance -= finalPayment;
  }
  if (balance > 0) return null;

  const first =
    dueDay !== null ? nextDueByDay(dueDay, today) : localDayAsUtc(today);
  const firstDate = new Date(first);
  const target = new Date(
    Date.UTC(firstDate.getUTCFullYear(), firstDate.getUTCMonth() + months - 1, 1),
  );
  const lastDay = new Date(
    Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0),
  ).getUTCDate();
  const payoffDate = Date.UTC(
    target.getUTCFullYear(),
    target.getUTCMonth(),
    Math.min(dueDay ?? firstDate.getUTCDate(), lastDay),
  );
  return { months, totalInterest, finalPayment, payoffDate };
}

/** "3 years 2 months", "7 months", "1 month". */
export function formatDuration(months: number): string {
  const years = Math.floor(months / 12);
  const rest = months % 12;
  const parts: string[] = [];
  if (years > 0) parts.push(`${years} ${years === 1 ? "year" : "years"}`);
  if (rest > 0 || years === 0) {
    parts.push(`${rest} ${rest === 1 ? "month" : "months"}`);
  }
  return parts.join(" ");
}
