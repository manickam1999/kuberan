"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Wallet, Plus } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useAccounts } from "@/hooks/use-accounts";
import { useBudgets } from "@/hooks/use-budgets";
import { useConnectedApps } from "@/hooks/use-oauth-clients";
import { useTransactions } from "@/hooks/use-transactions";
import { useBaseCurrency } from "@/hooks/use-profile";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CreateAccountDialog } from "@/components/accounts/create-account-dialog";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { NetWorthChart } from "@/components/dashboard/net-worth-chart";
import { HomeSpendCard } from "@/components/dashboard/home-spend-card";
import { HomeCashflowCard } from "@/components/dashboard/home-cashflow-card";
import { BudgetsCard } from "@/components/dashboard/budgets-card";
import { HomeRecent } from "@/components/dashboard/home-recent";
import { HomeAccounts } from "@/components/dashboard/home-accounts";
import { Eyebrow } from "@/components/dashboard/home-bits";
import { Welcome } from "@/components/dashboard/welcome";
import { needsOnboarding, onboardingSteps } from "@/lib/home";
import type { Transaction } from "@/types/models";

function HomeSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-[400px] w-full md:h-[340px]" />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <Skeleton className="h-[260px] w-full" />
        <Skeleton className="h-[260px] w-full" />
        <Skeleton className="h-[260px] w-full md:col-span-2 xl:col-span-1" />
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <Skeleton className="h-[380px] w-full" />
        <Skeleton className="h-[300px] w-full" />
      </div>
    </div>
  );
}

function greeting(now: Date): string {
  const h = now.getHours();
  if (h < 5) return "Good evening";
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

export default function DashboardPage() {
  const { user } = useAuth();
  const baseCurrency = useBaseCurrency();
  const [accountDialogOpen, setAccountDialogOpen] = useState(false);
  const [txDialogOpen, setTxDialogOpen] = useState(false);
  const [editTxOpen, setEditTxOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] =
    useState<Transaction | null>(null);

  const accountsQuery = useAccounts({ page_size: 100 });
  const accounts = useMemo(
    () => accountsQuery.data?.data ?? [],
    [accountsQuery.data]
  );
  // One list serves Recent (the newest few) and the onboarding check, which
  // needs to see past opening balances to any real transaction.
  const transactionsQuery = useTransactions({ page_size: 25 });
  const transactions = useMemo(
    () => transactionsQuery.data?.data ?? [],
    [transactionsQuery.data]
  );
  const budgetsQuery = useBudgets({ page_size: 1 });
  const appsQuery = useConnectedApps();

  const steps = useMemo(
    () =>
      onboardingSteps({
        accounts,
        transactions,
        budgetCount: budgetsQuery.data?.total_items ?? 0,
        connectedAppCount: appsQuery.data?.length ?? 0,
      }),
    [accounts, transactions, budgetsQuery.data, appsQuery.data]
  );

  const now = new Date();
  const today = now.toLocaleDateString("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });

  const loading = accountsQuery.isLoading || transactionsQuery.isLoading;
  const showWelcome =
    !loading &&
    !accountsQuery.error &&
    !transactionsQuery.error &&
    needsOnboarding(steps);

  return (
    <>
      {showWelcome ? (
        <Welcome
          firstName={user?.first_name}
          steps={steps}
          accounts={accounts}
          onAddAccount={() => setAccountDialogOpen(true)}
          onAddTransaction={() => setTxDialogOpen(true)}
        />
      ) : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <Eyebrow>{today}</Eyebrow>
              <h1 className="font-display text-3xl leading-tight max-md:hidden">
                {user?.first_name
                  ? `${greeting(now)}, ${user.first_name}`
                  : "Home"}
              </h1>
            </div>
            <div className="hidden gap-2 md:flex">
              <Button asChild variant="outline" size="sm">
                <Link href="/accounts">
                  <Wallet className="size-4" />
                  Add account
                </Link>
              </Button>
              <Button size="sm" onClick={() => setTxDialogOpen(true)}>
                <Plus className="size-4" />
                Add transaction
              </Button>
            </div>
          </div>

          {loading ? (
            <HomeSkeleton />
          ) : accountsQuery.error ? (
            <div className="rounded-card border p-10 text-center">
              <p className="text-sm text-destructive">
                Could not load your accounts.
              </p>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => accountsQuery.refetch()}
              >
                Try again
              </Button>
            </div>
          ) : (
            <>
              <NetWorthChart />

              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                <HomeSpendCard />
                <BudgetsCard />
                <div className="h-full md:col-span-2 xl:col-span-1">
                  <HomeCashflowCard />
                </div>
              </div>

              <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
                <HomeRecent
                  transactions={transactions.slice(0, 6)}
                  accounts={accounts}
                  baseCurrency={baseCurrency}
                  isLoading={false}
                  error={transactionsQuery.error}
                  onSelect={(tx) => {
                    setSelectedTransaction(tx);
                    setEditTxOpen(true);
                  }}
                />
                <HomeAccounts accounts={accounts} />
              </div>
            </>
          )}
        </div>
      )}

      <CreateAccountDialog
        open={accountDialogOpen}
        onOpenChange={setAccountDialogOpen}
      />
      <CreateTransactionDialog
        open={txDialogOpen}
        onOpenChange={setTxDialogOpen}
      />
      <EditTransactionDialog
        open={editTxOpen}
        onOpenChange={setEditTxOpen}
        transaction={selectedTransaction}
      />
    </>
  );
}
