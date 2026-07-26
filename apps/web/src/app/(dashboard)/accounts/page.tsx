"use client";

import { useState } from "react";
import Link from "next/link";
import { Plus, Pin, ArrowRight, Coins, Scale, Wallet } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAccounts, accountKeys } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import { apiClient } from "@/lib/api-client";
import { formatCurrency, formatPercentage } from "@/lib/format";
import { accountVisual } from "@/lib/domain-visuals";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { CreateAccountDialog } from "@/components/accounts/create-account-dialog";
import type { Account, AccountType } from "@/types/models";

const GROUP_ORDER: AccountType[] = ["cash", "investment", "credit_card", "debt"];

function AccountTile({
  account,
  onTogglePin,
  hideBalances,
}: {
  account: Account;
  onTogglePin: (id: string, pinned: boolean) => void;
  hideBalances: boolean;
}) {
  const v = accountVisual(account.type);
  const Icon = v.icon;
  const isLiability = account.type === "credit_card" || account.type === "debt";
  const utilization =
    account.type === "credit_card" && account.credit_limit
      ? Math.min((account.balance / account.credit_limit) * 100, 100)
      : null;

  return (
    <Card className="group relative gap-0 overflow-hidden py-0 transition-colors hover:border-primary/40">
      <button
        type="button"
        aria-label={account.is_pinned ? "Unpin account" : "Pin account"}
        onClick={(e) => {
          e.preventDefault();
          onTogglePin(account.id, !account.is_pinned);
        }}
        className="absolute right-3 top-3 z-10 rounded-md p-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 data-[pinned=true]:opacity-100"
        data-pinned={account.is_pinned}
      >
        <Pin
          className={`size-4 ${account.is_pinned ? "fill-primary text-primary" : ""}`}
        />
      </button>
      <Link href={`/accounts/${account.id}`} className="block p-5">
        <div className="flex items-center gap-3">
          <span
            className={`flex size-10 shrink-0 items-center justify-center rounded-xl ${v.chip}`}
          >
            <Icon className="size-5" />
          </span>
          <div className="min-w-0 pr-6">
            <p className="truncate font-medium">{account.name}</p>
            <p className="truncate text-xs text-muted-foreground">
              {account.broker ? account.broker : v.label}
              {!account.is_active && " · Inactive"}
            </p>
          </div>
        </div>

        <p
          className={`money mt-4 text-2xl font-semibold ${
            isLiability ? "text-warning" : ""
          }`}
        >
          {formatCurrency(account.balance, account.currency, hideBalances)}
        </p>

        {utilization !== null && account.credit_limit ? (
          <div className="mt-3 space-y-1.5">
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className={`h-full rounded-full ${
                  utilization > 75
                    ? "bg-negative"
                    : utilization > 50
                      ? "bg-warning"
                      : "bg-primary"
                }`}
                style={{ width: `${Math.max(utilization, 2)}%` }}
              />
            </div>
            <p className="text-xs text-muted-foreground">
              {formatPercentage(utilization)} of{" "}
              {formatCurrency(account.credit_limit, account.currency, hideBalances)} limit
            </p>
          </div>
        ) : (
          <p className="mt-3 flex items-center gap-1 text-xs text-muted-foreground">
            {account.currency}
            <span className="ml-auto inline-flex items-center gap-1 text-muted-foreground/70 opacity-0 transition-opacity group-hover:opacity-100">
              View <ArrowRight className="size-3" />
            </span>
          </p>
        )}
      </Link>
    </Card>
  );
}

function AccountsSkeleton() {
  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-40" />
        ))}
      </div>
    </div>
  );
}

export default function AccountsPage() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const { data, isLoading } = useAccounts({ page_size: 100 });
  const { hideBalances } = useAuth();
  const queryClient = useQueryClient();

  const accounts = data?.data ?? [];

  const handleTogglePin = async (id: string, pinned: boolean) => {
    await apiClient.put(`/api/v1/accounts/${id}`, { is_pinned: pinned });
    queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
  };

  const sumTypes = (types: AccountType[]) =>
    accounts
      .filter((a) => types.includes(a.type))
      .reduce((s, a) => s + a.balance, 0);
  const assets = sumTypes(["cash", "investment"]);
  const liabilities = sumTypes(["credit_card", "debt"]);
  const netWorth = assets - liabilities;

  const groups = GROUP_ORDER.map((type) => ({
    type,
    label: accountVisual(type).label,
    accounts: accounts
      .filter((a) => a.type === type)
      .sort((a, b) => Number(b.is_pinned) - Number(a.is_pinned) || Math.abs(b.balance) - Math.abs(a.balance)),
  })).filter((g) => g.accounts.length > 0);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Accounts</h1>
          <p className="text-sm text-muted-foreground">
            {accounts.length} account{accounts.length !== 1 ? "s" : ""} across{" "}
            {groups.length} type{groups.length !== 1 ? "s" : ""}
          </p>
        </div>
        <Button size="sm" onClick={() => setDialogOpen(true)}>
          <Plus className="size-4" />
          New account
        </Button>
      </div>

      {isLoading ? (
        <AccountsSkeleton />
      ) : accounts.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No accounts yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first account to get started.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" />
            Create account
          </Button>
        </div>
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <StatCard
              label="Total assets"
              value={formatCurrency(assets, undefined, hideBalances)}
              icon={Coins}
              tone="primary"
              sub="Cash + investments"
            />
            <StatCard
              label="Liabilities"
              value={formatCurrency(liabilities, undefined, hideBalances)}
              icon={Scale}
              tone="warning"
              sub="Credit cards + debt"
            />
            <StatCard
              label="Net worth"
              value={formatCurrency(netWorth, undefined, hideBalances)}
              icon={Wallet}
              tone="default"
              sub="Assets − liabilities"
            />
          </div>

          <div className="space-y-8">
            {groups.map((group) => {
              const subtotal = group.accounts.reduce(
                (s, a) => s + a.balance,
                0
              );
              return (
                <section key={group.type} className="space-y-3">
                  <div className="flex items-baseline justify-between border-b border-border/60 pb-2">
                    <h2 className="flex items-center gap-2 text-sm font-medium">
                      {group.label}
                      <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                        {group.accounts.length}
                      </span>
                    </h2>
                    <span className="money text-sm text-muted-foreground">
                      {formatCurrency(subtotal, undefined, hideBalances)}
                    </span>
                  </div>
                  <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                    {group.accounts.map((account) => (
                      <AccountTile
                        key={account.id}
                        account={account}
                        onTogglePin={handleTogglePin}
                        hideBalances={hideBalances}
                      />
                    ))}
                  </div>
                </section>
              );
            })}
          </div>
        </>
      )}

      <CreateAccountDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}
