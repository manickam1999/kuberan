"use client";

import { useState } from "react";
import Link from "next/link";
import { Wallet, Plus, ArrowRight } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useAccounts } from "@/hooks/use-accounts";
import { useTransactions } from "@/hooks/use-transactions";
import { formatCurrency, formatDate } from "@/lib/format";
import { accountVisual, transactionVisual } from "@/lib/domain-visuals";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { NetWorthChart } from "@/components/dashboard/net-worth-chart";
import { CompositionCard } from "@/components/dashboard/composition-card";
import { SpendingCard } from "@/components/dashboard/spending-card";
import { CashflowCard } from "@/components/dashboard/cashflow-card";
import { BudgetsCard } from "@/components/dashboard/budgets-card";
import { TodayExpensesCard } from "@/components/dashboard/today-expenses-card";
import type { Account, Transaction } from "@/types/models";

function DashboardSkeleton() {
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <div className="space-y-4 lg:col-span-2">
        <Skeleton className="h-[340px] w-full" />
        <Skeleton className="h-[260px] w-full" />
        <Skeleton className="h-[220px] w-full" />
      </div>
      <div className="space-y-4">
        <Skeleton className="h-[240px] w-full" />
        <Skeleton className="h-[240px] w-full" />
        <Skeleton className="h-[380px] w-full" />
      </div>
    </div>
  );
}

function SectionCardHeader({
  title,
  href,
}: {
  title: string;
  href: string;
}) {
  return (
    <CardHeader className="flex-row items-center justify-between space-y-0">
      <CardTitle className="text-base">{title}</CardTitle>
      <Button
        asChild
        variant="link"
        size="sm"
        className="h-auto p-0 text-muted-foreground"
      >
        <Link href={href}>
          View all <ArrowRight className="ml-1 size-3.5" />
        </Link>
      </Button>
    </CardHeader>
  );
}

function AccountsPanel({ accounts }: { accounts: Account[] }) {
  const pinned = accounts.filter((a) => a.is_pinned);
  const shown = (pinned.length > 0 ? pinned : [...accounts])
    .sort((a, b) => Math.abs(b.balance) - Math.abs(a.balance))
    .slice(0, 5);

  return (
    <Card>
      <SectionCardHeader
        title={pinned.length > 0 ? "Pinned accounts" : "Top accounts"}
        href="/accounts"
      />
      <CardContent className="space-y-0.5">
        {shown.map((account) => {
          const v = accountVisual(account.type);
          const Icon = v.icon;
          return (
            <Link
              key={account.id}
              href={`/accounts/${account.id}`}
              className="-mx-2 flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-accent/50"
            >
              <span
                className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${v.chip}`}
              >
                <Icon className="size-4" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{account.name}</p>
                <p className="text-xs text-muted-foreground">{v.label}</p>
              </div>
              <span className="money shrink-0 text-sm font-medium">
                {formatCurrency(account.balance, account.currency)}
              </span>
            </Link>
          );
        })}
      </CardContent>
    </Card>
  );
}

function TransactionRow({
  transaction,
  accountName,
  onClick,
}: {
  transaction: Transaction;
  accountName?: string;
  onClick?: () => void;
}) {
  const v = transactionVisual(transaction.type);
  const Icon = v.icon;

  return (
    <button
      type="button"
      className="-mx-2 flex w-full items-center justify-between gap-3 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent/50"
      onClick={onClick}
    >
      <div className="flex min-w-0 items-center gap-3">
        <span
          className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${v.chip}`}
        >
          <Icon className="size-4" />
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">
            {transaction.description || v.label}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {formatDate(transaction.date)}
            {accountName && ` · ${accountName}`}
          </p>
        </div>
      </div>
      <span className={`money shrink-0 text-sm font-semibold ${v.text}`}>
        {v.sign}
        {formatCurrency(transaction.amount)}
      </span>
    </button>
  );
}

export default function DashboardPage() {
  const { user } = useAuth();
  const [txDialogOpen, setTxDialogOpen] = useState(false);
  const [editTxOpen, setEditTxOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] =
    useState<Transaction | null>(null);
  const { data: accountsData, isLoading: accountsLoading } = useAccounts();

  const accounts = accountsData?.data ?? [];

  const { data: transactionsData, isLoading: transactionsLoading } =
    useTransactions({ page_size: 5 });
  const transactions = transactionsData?.data ?? [];

  const accountNameMap = new Map(accounts.map((a) => [a.id, a.name]));

  const today = new Date().toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
  });

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {user?.first_name ? `Welcome back, ${user.first_name}` : "Dashboard"}
          </h1>
          <p className="text-sm text-muted-foreground">{today}</p>
        </div>
        <div className="flex gap-2">
          <Button asChild variant="outline" size="sm">
            <Link href="/accounts">
              <Wallet className="size-4" />
              <span className="hidden sm:inline">Add account</span>
            </Link>
          </Button>
          <Button size="sm" onClick={() => setTxDialogOpen(true)}>
            <Plus className="size-4" />
            <span className="hidden sm:inline">Add transaction</span>
          </Button>
        </div>
      </div>

      {accountsLoading ? (
        <DashboardSkeleton />
      ) : accounts.length === 0 ? (
        <Card className="surface-glow">
          <CardHeader>
            <CardTitle>Get started</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Create your first account to start tracking your finances.
            </p>
            <Button asChild>
              <Link href="/accounts">
                <Plus className="size-4" />
                Create account
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-3">
          {/* Main column */}
          <div className="min-w-0 space-y-4 lg:col-span-2">
            <NetWorthChart />
            <SpendingCard />
            <CashflowCard />
            <BudgetsCard />
          </div>

          {/* Rail */}
          <div className="min-w-0 space-y-4">
            <CompositionCard />
            <TodayExpensesCard />

            <Card>
              <SectionCardHeader
                title="Recent activity"
                href="/transactions"
              />
              <CardContent>
                {transactionsLoading ? (
                  <div className="space-y-2">
                    {Array.from({ length: 5 }).map((_, i) => (
                      <Skeleton key={i} className="h-11 w-full" />
                    ))}
                  </div>
                ) : transactions.length === 0 ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">
                    No transactions yet.
                  </p>
                ) : (
                  <div className="divide-y divide-border/60">
                    {transactions.map((tx) => (
                      <TransactionRow
                        key={tx.id}
                        transaction={tx}
                        accountName={accountNameMap.get(tx.account_id)}
                        onClick={() => {
                          setSelectedTransaction(tx);
                          setEditTxOpen(true);
                        }}
                      />
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>

            <AccountsPanel accounts={accounts} />
          </div>
        </div>
      )}

      <CreateTransactionDialog
        open={txDialogOpen}
        onOpenChange={setTxDialogOpen}
      />
      <EditTransactionDialog
        open={editTxOpen}
        onOpenChange={setEditTxOpen}
        transaction={selectedTransaction}
      />
    </div>
  );
}
