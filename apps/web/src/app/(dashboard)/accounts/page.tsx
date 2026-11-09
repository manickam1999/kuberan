"use client";

import { useMemo, useState } from "react";
import { Plus, Wallet } from "lucide-react";
import { useAccounts } from "@/hooks/use-accounts";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useReceivablesSummary } from "@/hooks/use-receivables";
import { WealthSwitch } from "@/components/layout/wealth-switch";
import { AccountRow } from "@/components/accounts/account-row";
import { CreateAccountDialog } from "@/components/accounts/create-account-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { FxNotice } from "@/components/ui/fx-notice";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import { ToggleChip } from "@/components/ui/toggle-chip";
import { buildAccountsOverview } from "@/lib/account-math";

function AccountsSkeleton() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading accounts">
      <Skeleton className="h-[74px] w-full rounded-card" />
      {[2, 1, 1].map((rows, g) => (
        <div key={g} className="space-y-2">
          <Skeleton className="h-5 w-40" />
          <Card className="gap-0 divide-y py-0">
            {Array.from({ length: rows }).map((_, i) => (
              <div key={i} className="flex items-center gap-3 px-4 py-3">
                <Skeleton className="size-[42px] rounded-xl" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-3 w-28" />
                </div>
                <Skeleton className="h-4 w-24" />
              </div>
            ))}
          </Card>
        </div>
      ))}
    </div>
  );
}

export default function AccountsPage() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const { data, isLoading, isError, refetch } = useAccounts({ page_size: 100 });
  const baseCurrency = useBaseCurrency();
  const { data: owed } = useReceivablesSummary();

  const accounts = useMemo(() => data?.data ?? [], [data]);
  const overview = useMemo(
    () => buildAccountsOverview(accounts, baseCurrency, showInactive),
    [accounts, baseCurrency, showInactive],
  );
  const today = useMemo(() => new Date(), []);

  const currencies = new Set(
    accounts.filter((a) => a.is_active).map((a) => a.currency),
  );
  const activeCount = accounts.length - overview.inactiveCount;
  const sub = `${activeCount} account${activeCount === 1 ? "" : "s"}${
    currencies.size === 1
      ? ` · all in ${Array.from(currencies)[0]}`
      : currencies.size > 1
        ? ` · ${currencies.size} currencies`
        : ""
  }`;

  // With no convertible account every base total would read 0: show a dash.
  const hasTotals = overview.convertedCount > 0;
  const stats: StatStripItem[] = [
    {
      label: "Assets",
      value: hasTotals ? (
        <Money amount={overview.assets} currency={baseCurrency} />
      ) : (
        "—"
      ),
    },
    {
      label: "Liabilities",
      value: hasTotals ? (
        <Money amount={-overview.liabilities} currency={baseCurrency} />
      ) : (
        "—"
      ),
    },
    {
      label: "Net worth",
      value: hasTotals ? (
        <Money amount={overview.netWorth} currency={baseCurrency} />
      ) : (
        "—"
      ),
    },
  ];
  if (owed && owed.total_outstanding > 0) {
    stats.push({
      label: "Owed to you",
      value: <Money amount={owed.total_outstanding} currency={owed.currency} />,
      sub: "Not in net worth",
    });
  }

  return (
    <div className="space-y-6">
      <WealthSwitch current="/accounts" />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Accounts
          </h1>
          {!isLoading && !isError && accounts.length > 0 && (
            <p className="text-muted-foreground text-sm">{sub}</p>
          )}
        </div>
        <Button size="sm" onClick={() => setDialogOpen(true)}>
          <Plus className="size-4" />
          New account
        </Button>
      </div>

      {isLoading ? (
        <AccountsSkeleton />
      ) : isError ? (
        <div
          role="alert"
          className="flex flex-col items-center gap-3 rounded-card border border-dashed p-10 text-center"
        >
          <h2 className="text-lg font-semibold">
            Couldn&apos;t load your accounts
          </h2>
          <p className="text-muted-foreground text-sm">
            Check your connection and try again.
          </p>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            Try again
          </Button>
        </div>
      ) : accounts.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-1 rounded-card border border-dashed p-12 text-center">
          <Wallet aria-hidden className="text-muted-foreground mb-2 size-6" />
          <h2 className="text-lg font-semibold">No accounts yet</h2>
          <p className="text-muted-foreground text-sm">
            Add a cash, card, investment or fixed deposit account to start
            tracking.
          </p>
          <Button
            className="mt-4"
            size="sm"
            onClick={() => setDialogOpen(true)}
          >
            <Plus className="size-4" />
            Create account
          </Button>
        </div>
      ) : (
        <>
          <FxNotice
            unconverted={overview.unconverted}
            fxStale={overview.stale}
          />
          <StatStrip items={stats} />

          <div className="space-y-6">
            {overview.groups.map((group) => (
              <section key={group.key} aria-labelledby={`grp-${group.key}`}>
                <div className="mb-2 flex items-baseline gap-2 px-1">
                  <h2 id={`grp-${group.key}`} className="text-sm font-semibold">
                    {group.label}
                  </h2>
                  <span className="text-muted-foreground text-xs">
                    {group.accounts.length}
                  </span>
                  <span className="money text-muted-foreground ml-auto text-sm">
                    {group.convertedCount > 0 ? (
                      <Money amount={group.totalBase} currency={baseCurrency} />
                    ) : (
                      "—"
                    )}
                  </span>
                </div>
                <Card className="gap-0 divide-y overflow-hidden py-0">
                  {group.accounts.map((account) => (
                    <AccountRow
                      key={account.id}
                      account={account}
                      today={today}
                    />
                  ))}
                </Card>
              </section>
            ))}
          </div>

          {overview.inactiveCount > 0 && (
            <div className="flex justify-center">
              <ToggleChip
                pressed={showInactive}
                onPressedChange={setShowInactive}
              >
                {showInactive ? "Hide" : "Show"} inactive accounts (
                {overview.inactiveCount})
              </ToggleChip>
            </div>
          )}
        </>
      )}

      <CreateAccountDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}
