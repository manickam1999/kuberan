"use client";

import Link from "next/link";
import { PriceSparkline } from "@/components/investments/price-sparkline";
import { HomeCard } from "@/components/dashboard/home-bits";
import { Money } from "@/components/ui/money";
import { Tile } from "@/components/ui/tile";
import { useAccountSparklines } from "@/hooks/use-accounts";
import { useBaseCurrency } from "@/hooks/use-profile";
import { accountVisual } from "@/lib/domain-visuals";
import type { Account } from "@/types/models";

const SHOWN = 5;

const ACCOUNT_TONE = {
  cash: "accent",
  investment: "investment",
  credit_card: "warn",
  debt: "neg",
  fixed_deposit: "info",
} as const;

/** Pinned accounts, else the largest by base-currency balance. */
export function HomeAccounts({ accounts }: { accounts: readonly Account[] }) {
  const baseCurrency = useBaseCurrency();
  const { data: sparklines } = useAccountSparklines(30);
  // Rank by the base-currency balance; accounts with no rate sort last.
  const baseOf = (a: Account): number | undefined =>
    a.balance_base ?? (a.currency === baseCurrency ? a.balance : undefined);
  const active = accounts.filter((a) => a.is_active);
  const pinned = active.filter((a) => a.is_pinned);
  const shown = [...(pinned.length > 0 ? pinned : active)]
    .sort((a, b) => Math.abs(baseOf(b) ?? 0) - Math.abs(baseOf(a) ?? 0))
    .slice(0, SHOWN);

  return (
    <HomeCard title="Accounts" href="/accounts" linkLabel="Manage">
      <div className="space-y-0.5">
        {shown.map((account) => {
          const v = accountVisual(account.type);
          const Icon = v.icon;
          const sub =
            account.currency !== baseCurrency
              ? `${v.label} · ${account.currency}`
              : v.label;
          return (
            <Link
              key={account.id}
              href={`/accounts/${account.id}`}
              className="-mx-2 flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-accent/50 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <Tile tone={ACCOUNT_TONE[account.type] ?? "accent"}>
                <Icon />
              </Tile>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">
                  {account.name}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {sub}
                </span>
              </span>
              {(sparklines?.get(account.id)?.length ?? 0) > 1 && (
                <PriceSparkline
                  values={sparklines?.get(account.id) ?? []}
                  tone="neutral"
                  width={56}
                  height={22}
                />
              )}
              <span className="min-w-28 shrink-0 text-right text-sm font-medium">
                <Money
                  amount={account.balance}
                  currency={account.currency}
                  baseAmount={account.balance_base}
                  baseCurrency={baseCurrency}
                  fxRate={account.fx_rate}
                  fxStale={account.fx_stale}
                  fxMissing={account.fx_missing}
                  layout="stacked"
                />
              </span>
            </Link>
          );
        })}
      </div>
    </HomeCard>
  );
}
