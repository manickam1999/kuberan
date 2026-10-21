import type { Account, AccountType } from "@/types/models";

// Account types day-to-day spending is recorded against. Mirrors the backend
// AccountType.IsSpending.
const DEFAULT_ACCOUNT_TYPES: readonly AccountType[] = [
  "cash",
  "credit_card",
  "debt",
];

/** Whether an account may be chosen as the default for new transactions. */
export function canBeDefaultAccount(account: Account): boolean {
  return account.is_active && DEFAULT_ACCOUNT_TYPES.includes(account.type);
}

/**
 * Resolves the user's saved default account against the loaded account list,
 * ignoring it if the account has since been deactivated or deleted.
 */
export function resolveDefaultAccountId(
  defaultAccountId: string | null | undefined,
  accounts: Account[]
): string {
  if (!defaultAccountId) return "";
  const account = accounts.find((a) => a.id === defaultAccountId);
  return account && canBeDefaultAccount(account) ? account.id : "";
}
