import Link from "next/link";
import { ChevronRight, Pin } from "lucide-react";
import { Money } from "@/components/ui/money";
import { Tile, type TileTone } from "@/components/ui/tile";
import { accountVisual } from "@/lib/domain-visuals";
import { formatDate } from "@/lib/format";
import {
  daysUntil,
  isLiability,
  nextDueDate,
  signedBalance,
  utilisation,
} from "@/lib/account-math";
import { cn } from "@/lib/utils";
import type { Account, AccountType } from "@/types/models";

const TILE_TONE: Record<AccountType, TileTone> = {
  cash: "accent",
  investment: "investment",
  credit_card: "warn",
  debt: "neg",
  fixed_deposit: "info",
};

/** The muted second line of an account row. */
function subtitle(
  account: Account,
  today: Date,
): { text: string; warn: boolean } {
  const label = accountVisual(account.type).label;
  const parts: string[] = [label];
  let warn = false;

  if (account.type === "investment" && account.broker) {
    parts[0] = account.broker;
  }
  if (account.type === "fixed_deposit" && account.fixed_deposit) {
    const fd = account.fixed_deposit;
    if (fd.status === "matured") parts.push("Matured");
    else if (fd.status === "closed") parts.push("Closed");
    else parts.push(`Matures ${formatDate(fd.maturity_date)}`);
  }
  if (account.type === "credit_card") {
    const due = nextDueDate(account.due_date, today);
    if (due !== null) {
      const days = daysUntil(due, today);
      parts.push(`Due ${formatDate(new Date(due).toISOString())}`);
      if (days <= 7 && account.balance > 0) warn = true;
    }
    const used = utilisation(account.balance, account.credit_limit);
    if (used !== null) parts.push(`${Math.round(used)}% of limit`);
  }
  if (!account.is_active) parts.push("Inactive");
  return { text: parts.join(" · "), warn };
}

/**
 * One account in a balance-sheet group: tile, name and detail line on the
 * left, the balance in the account's own currency on the right (liabilities
 * read negative). The whole row links to the account.
 */
export function AccountRow({
  account,
  today,
}: {
  account: Account;
  today: Date;
}) {
  const v = accountVisual(account.type);
  const Icon = v.icon;
  const sub = subtitle(account, today);
  const liability = isLiability(account);

  return (
    <Link
      href={`/accounts/${account.id}`}
      className={cn(
        "hover:bg-muted/50 focus-visible:ring-ring/50 flex min-h-14 items-center gap-3 px-4 py-3 outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-inset",
        !account.is_active && "opacity-70",
      )}
    >
      <Tile tone={TILE_TONE[account.type]} size="lg">
        <Icon />
      </Tile>
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="flex items-center gap-1.5 text-sm font-medium">
          <span className="truncate">{account.name}</span>
          {account.is_pinned && (
            <Pin
              aria-label="Pinned"
              className="fill-primary text-primary size-3 shrink-0"
            />
          )}
        </span>
        <span
          className={cn(
            "truncate text-xs",
            sub.warn ? "text-warning" : "text-muted-foreground",
          )}
        >
          {sub.text}
        </span>
      </span>
      <span
        className={cn(
          "money shrink-0 text-right text-sm font-medium",
          liability && "text-warning",
        )}
      >
        <Money
          amount={signedBalance(account)}
          currency={account.currency}
          baseAmount={
            account.balance_base != null
              ? liability
                ? -account.balance_base
                : account.balance_base
              : undefined
          }
          fxRate={account.fx_rate}
          fxStale={account.fx_stale}
          fxMissing={account.fx_missing}
          layout="stacked"
          className="items-end"
        />
      </span>
      <ChevronRight
        aria-hidden
        className="text-muted-foreground size-4 shrink-0"
      />
    </Link>
  );
}
