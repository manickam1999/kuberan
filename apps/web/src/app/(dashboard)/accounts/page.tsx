"use client";

import { useState } from "react";
import Link from "next/link";
import { Plus, Pin, ArrowRight, Coins, Scale, Wallet, Vault } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAccounts, accountKeys } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import { apiClient } from "@/lib/api-client";
import { formatCurrency, formatDate, formatPercentage } from "@/lib/format";
import { accountVisual } from "@/lib/domain-visuals";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { CreateAccountDialog } from "@/components/accounts/create-account-dialog";
import { CloseFixedDepositDialog } from "@/components/accounts/close-fixed-deposit-dialog";
import type { Account, AccountType } from "@/types/models";

const GROUP_ORDER: AccountType[] = ["cash", "investment", "fixed_deposit", "credit_card", "debt"];

function AccountTile({
  account,
  onTogglePin,
  hideBalances,
  onCloseFD,
}: {
  account: Account;
  onTogglePin: (id: string, pinned: boolean) => void;
  hideBalances: boolean;
  onCloseFD?: (account: Account) => void;
}) {
  const v = accountVisual(account.type);
  const Icon = v.icon;
  const isLiability = account.type === "credit_card" || account.type === "debt";
  const utilization =
    account.type === "credit_card" && account.credit_limit
      ? Math.min((account.balance / account.credit_limit) * 100, 100)
      : null;

  const fdProgress =
    account.type === "fixed_deposit" && account.fixed_deposit
      ? (() => {
          const start = new Date(account.fixed_deposit.start_date.split("T")[0]).getTime();
          const end = new Date(account.fixed_deposit.maturity_date.split("T")[0]).getTime();
          const now = Date.now();
          return Math.min(Math.max(((now - start) / (end - start)) * 100, 0), 100);
        })()
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

        {account.type === "fixed_deposit" && account.fixed_deposit && (
          <>
            {account.fixed_deposit.status === "matured" && (
              <span className="mt-1 inline-flex items-center rounded-full bg-info-muted px-2 py-0.5 text-xs font-medium text-info">
                Matured
              </span>
            )}
            {fdProgress !== null && (
              <div className="mt-3 space-y-1.5">
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-info"
                    style={{ width: `${Math.max(fdProgress, 2)}%` }}
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  Matures {formatDate(account.fixed_deposit.maturity_date)} ·{" "}
                  {formatCurrency(account.fixed_deposit.maturity_value, account.currency, hideBalances)} projected
                </p>
              </div>
            )}
            {account.fixed_deposit.status !== "closed" && onCloseFD && (
              <button
                type="button"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  onCloseFD(account);
                }}
                className="mt-2 text-xs text-info hover:underline"
              >
                {account.fixed_deposit.status === "matured" ? "Record payout →" : "Close early →"}
              </button>
            )}
          </>
        )}

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
        ) : account.type !== "fixed_deposit" ? (
          <p className="mt-3 flex items-center gap-1 text-xs text-muted-foreground">
            {account.currency}
            <span className="ml-auto inline-flex items-center gap-1 text-muted-foreground/70 opacity-0 transition-opacity group-hover:opacity-100">
              View <ArrowRight className="size-3" />
            </span>
          </p>
        ) : null}
      </Link>
    </Card>
  );
}

function AccountsSkeleton() {
  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
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
  const [closeDialogAccount, setCloseDialogAccount] = useState<Account | null>(null);
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
  const assets = sumTypes(["cash", "investment", "fixed_deposit"]);
  const fixedDeposits = sumTypes(["fixed_deposit"]);
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
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Total assets"
              value={formatCurrency(assets, undefined, hideBalances)}
              icon={Coins}
              tone="primary"
              sub="Cash + investments + FDs"
            />
            <StatCard
              label="Fixed deposits"
              value={formatCurrency(fixedDeposits, undefined, hideBalances)}
              icon={Vault}
              tone="info"
              sub="Locked term deposits"
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
                        onCloseFD={setCloseDialogAccount}
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

      {closeDialogAccount && (
        <CloseFixedDepositDialog
          account={closeDialogAccount}
          open={!!closeDialogAccount}
          onOpenChange={(open) => {
            if (!open) setCloseDialogAccount(null);
          }}
        />
      )}
    </div>
  );
}
