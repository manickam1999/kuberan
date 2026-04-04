"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Wallet,
  ArrowLeftRight,
  Plus,
  ArrowUpRight,
  ArrowDownRight,
  TrendingUp,
  Landmark,
  PiggyBank,
  CreditCard,
  Pin,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/hooks/use-auth";
import { useAccounts, accountKeys } from "@/hooks/use-accounts";
import { apiClient } from "@/lib/api-client";
import { useTransactions } from "@/hooks/use-transactions";
import { useBudgets, useBudgetProgress } from "@/hooks/use-budgets";
import { usePortfolio } from "@/hooks/use-investments";
import { formatCurrency, formatDate, formatPercentage } from "@/lib/format";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { ExpenditureChart } from "@/components/dashboard/expenditure-chart";
import { IncomeExpensesChart } from "@/components/dashboard/income-expenses-chart";
import { SpendingTrendChart } from "@/components/dashboard/spending-trend-chart";
import { NetWorthChart } from "@/components/dashboard/net-worth-chart";
import type { Account, Budget, Transaction, TransactionType } from "@/types/models";

const ACCOUNT_TYPE_LABELS: Record<string, string> = {
  cash: "Cash",
  investment: "Investment",
  debt: "Debt",
  credit_card: "Credit Card",
};

const TRANSACTION_TYPE_CONFIG: Record<
  TransactionType,
  { label: string; color: string; icon: typeof ArrowUpRight }
> = {
  income: { label: "Income", color: "text-green-600", icon: ArrowUpRight },
  expense: { label: "Expense", color: "text-red-600", icon: ArrowDownRight },
  transfer: {
    label: "Transfer",
    color: "text-blue-600",
    icon: ArrowLeftRight,
  },
  investment: {
    label: "Investment",
    color: "text-purple-600",
    icon: TrendingUp,
  },
};

function DashboardSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-64" />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
      <Skeleton className="h-[250px] w-full" />
      <Skeleton className="h-[400px] w-full" />
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-[350px]" />
        <Skeleton className="h-[350px]" />
      </div>
      <Skeleton className="h-[250px] w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}

function SummaryCards({ accounts }: { accounts: Account[] }) {
  const { data: portfolio } = usePortfolio();

  const cashTotal = accounts
    .filter((a) => a.type === "cash")
    .reduce((sum, a) => sum + a.balance, 0);
  const investmentAccountTotal = accounts
    .filter((a) => a.type === "investment")
    .reduce((sum, a) => sum + a.balance, 0);
  const creditCardTotal = accounts
    .filter((a) => a.type === "credit_card")
    .reduce((sum, a) => sum + a.balance, 0);

  // Use real portfolio value when available, fall back to account balances
  const investmentValue = portfolio?.total_value ?? investmentAccountTotal;
  const netWorth = cashTotal + investmentValue - creditCardTotal;

  const cashCount = accounts.filter((a) => a.type === "cash").length;
  const creditCardCount = accounts.filter((a) => a.type === "credit_card").length;

  const holdingsCount = portfolio
    ? Object.values(portfolio.holdings_by_type).reduce((sum, h) => sum + h.count, 0)
    : 0;

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardDescription>Net Worth</CardDescription>
            <Wallet className="h-4 w-4 text-muted-foreground" />
          </div>
          <CardTitle className="text-3xl">{formatCurrency(netWorth)}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Across {accounts.length} account{accounts.length !== 1 ? "s" : ""}
          </p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardDescription>Cash</CardDescription>
            <PiggyBank className="h-4 w-4 text-muted-foreground" />
          </div>
          <CardTitle className="text-3xl">{formatCurrency(cashTotal)}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {cashCount} cash account{cashCount !== 1 ? "s" : ""}
          </p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardDescription>Investments</CardDescription>
            <Landmark className="h-4 w-4 text-muted-foreground" />
          </div>
          <CardTitle className="text-3xl">
            {formatCurrency(investmentValue)}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {portfolio ? (
            <div className="space-y-0.5">
              <p
                className={`text-sm font-medium ${portfolio.total_gain_loss >= 0 ? "text-green-600" : "text-red-600"}`}
              >
                {portfolio.total_gain_loss >= 0 ? "+" : ""}
                {formatCurrency(portfolio.total_gain_loss)} (
                {portfolio.total_gain_loss >= 0 ? "+" : ""}
                {formatPercentage(portfolio.gain_loss_pct)})
              </p>
              <p className="text-xs text-muted-foreground">
                {holdingsCount} holding{holdingsCount !== 1 ? "s" : ""}
              </p>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {accounts.filter((a) => a.type === "investment").length} investment
              account
              {accounts.filter((a) => a.type === "investment").length !== 1 ? "s" : ""}
            </p>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardDescription>Credit Cards</CardDescription>
            <CreditCard className="h-4 w-4 text-muted-foreground" />
          </div>
          <CardTitle className="text-3xl text-orange-600 dark:text-orange-400">
            {formatCurrency(creditCardTotal)}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {creditCardCount} credit card{creditCardCount !== 1 ? "s" : ""}
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function AccountCard({
  account,
  onTogglePin,
}: {
  account: Account;
  onTogglePin?: (accountId: string, pinned: boolean) => void;
}) {
  return (
    <Link href={`/accounts/${account.id}`}>
      <Card className="transition-colors hover:bg-accent/50">
        <CardHeader>
          <div className="flex items-center justify-between gap-2 min-w-0">
            <CardTitle className="text-base truncate min-w-0">{account.name}</CardTitle>
            <div className="flex items-center gap-1 shrink-0">
              {onTogglePin && (
                <button
                  type="button"
                  className="p-1 rounded-md hover:bg-muted transition-colors"
                  aria-label={account.is_pinned ? "Unpin account" : "Pin account"}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    onTogglePin(account.id, !account.is_pinned);
                  }}
                >
                  <Pin
                    className={`h-3.5 w-3.5 ${account.is_pinned ? "fill-primary text-primary" : "text-muted-foreground"}`}
                  />
                </button>
              )}
              <Badge variant="secondary">
                {ACCOUNT_TYPE_LABELS[account.type] ?? account.type}
              </Badge>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-2xl font-semibold">
            {formatCurrency(account.balance, account.currency)}
          </p>
        </CardContent>
      </Card>
    </Link>
  );
}

function BudgetMiniProgress({ budgetId }: { budgetId: string }) {
  const { data: progress, isLoading } = useBudgetProgress(budgetId);

  if (isLoading) {
    return <Skeleton className="h-2 w-full rounded-full" />;
  }

  if (!progress) return null;

  const pct = Math.min(progress.percentage, 100);
  const barColor =
    progress.percentage > 90
      ? "bg-red-500"
      : progress.percentage > 75
        ? "bg-amber-500"
        : "bg-emerald-500";

  return (
    <div className="space-y-1">
      <div className="h-2 w-full rounded-full bg-muted">
        <div
          className={`h-2 rounded-full transition-all ${barColor}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {formatCurrency(progress.spent)} / {formatCurrency(progress.budgeted)}
      </p>
    </div>
  );
}

function BudgetOverview({ budgets }: { budgets: Budget[] }) {
  if (budgets.length === 0) return null;

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Budget Overview</h2>
        <Button asChild variant="link" size="sm">
          <Link href="/budgets">View all</Link>
        </Button>
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {budgets.slice(0, 4).map((budget) => (
          <Card key={budget.id}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                {budget.name}
              </CardTitle>
              {budget.category && (
                <Badge variant="outline" className="w-fit text-xs">
                  {budget.category.name}
                </Badge>
              )}
            </CardHeader>
            <CardContent>
              <BudgetMiniProgress budgetId={budget.id} />
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
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
  const config = TRANSACTION_TYPE_CONFIG[transaction.type];
  const Icon = config.icon;
  const isNegative =
    transaction.type === "expense" || transaction.type === "transfer";

  return (
    <div
      className={`flex items-center justify-between py-3${onClick ? " cursor-pointer hover:bg-accent/50 rounded-md px-2 -mx-2" : ""}`}
      onClick={onClick}
    >
      <div className="flex items-center gap-3">
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted">
          <Icon className={`h-4 w-4 ${config.color}`} />
        </div>
        <div>
          <p className="text-sm font-medium">
            {transaction.description || config.label}
          </p>
          <p className="text-xs text-muted-foreground">
            {formatDate(transaction.date)}
            {accountName && ` · ${accountName}`}
          </p>
        </div>
      </div>
      <span className={`text-sm font-medium ${config.color}`}>
        {isNegative ? "-" : "+"}
        {formatCurrency(transaction.amount)}
      </span>
    </div>
  );
}

export default function DashboardPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const [txDialogOpen, setTxDialogOpen] = useState(false);
  const [editTxOpen, setEditTxOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null);
  const { data: accountsData, isLoading: accountsLoading } = useAccounts();

  const handleTogglePin = async (accountId: string, pinned: boolean) => {
    await apiClient.put(`/api/v1/accounts/${accountId}`, { is_pinned: pinned });
    queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
  };

  const accounts = accountsData?.data ?? [];

  const { data: transactionsData, isLoading: transactionsLoading } =
    useTransactions({ page_size: 5 });

  const transactions = transactionsData?.data ?? [];

  const { data: budgetsData } = useBudgets({
    is_active: true,
    page_size: 4,
  });
  const activeBudgets = budgetsData?.data ?? [];

  // Build account name lookup for transaction display
  const accountNameMap = new Map(accounts.map((a) => [a.id, a.name]));

  if (accountsLoading) {
    return <DashboardSkeleton />;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">
          {user?.first_name
            ? `Welcome back, ${user.first_name}`
            : "Dashboard"}
        </h1>
        <div className="flex flex-row gap-2">
          {/* Add Account - Mobile: Icon only */}
          <Button asChild variant="outline" className="sm:hidden" size="icon-sm" aria-label="Add Account">
            <Link href="/accounts">
              <Wallet />
            </Link>
          </Button>
          {/* Add Account - Desktop: Full text */}
          <Button asChild variant="outline" className="hidden sm:inline-flex" size="sm">
            <Link href="/accounts">
              <Wallet className="mr-2 h-4 w-4" />
              Add Account
            </Link>
          </Button>
          
          {/* Add Transaction - Mobile: Icon only */}
          <Button
            variant="outline"
            size="icon-sm"
            className="sm:hidden"
            onClick={() => setTxDialogOpen(true)}
            aria-label="Add Transaction"
          >
            <Plus />
          </Button>
          {/* Add Transaction - Desktop: Full text */}
          <Button
            variant="outline"
            size="sm"
            className="hidden sm:inline-flex"
            onClick={() => setTxDialogOpen(true)}
          >
            <Plus className="mr-2 h-4 w-4" />
            Add Transaction
          </Button>
        </div>
      </div>

      {accounts.length === 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>Get Started</CardTitle>
            <CardDescription>
              Create your first account to start tracking your finances.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link href="/accounts">
                <Plus className="mr-2 h-4 w-4" />
                Create Account
              </Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <>
          <SummaryCards accounts={accounts} />

          <div>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-lg font-semibold">Accounts</h2>
              <Button asChild variant="link" size="sm">
                <Link href="/accounts">View all</Link>
              </Button>
            </div>
            {accounts.some((a) => a.is_pinned) ? (
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {accounts
                  .filter((a) => a.is_pinned)
                  .map((account) => (
                    <AccountCard key={account.id} account={account} onTogglePin={handleTogglePin} />
                  ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No pinned accounts. Pin accounts from the{" "}
                <Link href="/accounts" className="underline">accounts page</Link>{" "}
                to see them here.
              </p>
            )}
          </div>

          <BudgetOverview budgets={activeBudgets} />

          <NetWorthChart />

          <div className="grid gap-4 lg:grid-cols-2">
            <ExpenditureChart />
            <IncomeExpensesChart />
          </div>
          <SpendingTrendChart />

          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Recent Transactions</CardTitle>
                <Button asChild variant="link" size="sm">
                  <Link href="/transactions">View all</Link>
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {transactionsLoading ? (
                <div className="space-y-3">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} className="h-12 w-full" />
                  ))}
                </div>
              ) : transactions.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No transactions yet. Add a transaction to see it here.
                </p>
              ) : (
                <div className="divide-y">
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
        </>
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
