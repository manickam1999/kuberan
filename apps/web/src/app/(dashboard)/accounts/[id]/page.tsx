"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ArrowLeftRight,
  ArrowUpRight,
  ArrowDownRight,
  Pencil,
  Plus,
  TrendingUp,
  ChevronLeft,
  ChevronRight,
  Pin,
  Loader2,
  Vault,
  HandCoins,
  Handshake,
} from "lucide-react";
import { useAccount, useUpdateAccount } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import { useAccountTransactions } from "@/hooks/use-transactions";
import { useAccountInvestments } from "@/hooks/use-investments";
import { useCategories } from "@/hooks/use-categories";
import { formatCurrency, formatDate, formatPayoutType, formatPayoutFrequency, formatCompounding } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  SharedKindBadge,
  YourShareText,
} from "@/components/receivables/shared-expense-indicators";
import { LendMoneyDialog } from "@/components/receivables/lend-money-dialog";
import { isSettlementAccountType } from "@/lib/receivable-format";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { AddInvestmentDialog } from "@/components/investments/add-investment-dialog";
import { CloseFixedDepositDialog } from "@/components/accounts/close-fixed-deposit-dialog";
import type { Transaction, TransactionType } from "@/types/models";
import type { TransactionFilters } from "@/types/api";

const ACCOUNT_TYPE_LABELS: Record<string, string> = {
  cash: "Cash",
  investment: "Investment",
  debt: "Debt",
  credit_card: "Credit Card",
  fixed_deposit: "Fixed deposit",
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
  // Repayment of a receivable: a credit to the account, never income.
  reimbursement: {
    label: "Repayment",
    color: "text-emerald-600",
    icon: HandCoins,
  },
};

const PAGE_SIZE = 20;

function AccountDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-6 w-32" />
      <div className="space-y-2">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-4 w-96" />
      </div>
      <Skeleton className="h-10 w-full" />
      <div className="space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    </div>
  );
}

function TransactionsTableRow({
  transaction,
  onClick,
}: {
  transaction: Transaction;
  onClick?: () => void;
}) {
  const config = TRANSACTION_TYPE_CONFIG[transaction.type];
  const Icon = config.icon;
  const isNegative =
    transaction.type === "expense" || transaction.type === "transfer";

  return (
    <TableRow className={onClick ? "cursor-pointer" : ""} onClick={onClick}>
      <TableCell className="text-muted-foreground">
        {formatDate(transaction.date)}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <div className="flex h-6 w-6 items-center justify-center rounded-full bg-muted">
            <Icon className={`h-3 w-3 ${config.color}`} />
          </div>
          {transaction.description || config.label}
          <SharedKindBadge transaction={transaction} />
        </div>
      </TableCell>
      <TableCell>
        <Badge variant="outline" className={config.color}>
          {config.label}
        </Badge>
      </TableCell>
      <TableCell>
        {transaction.category?.name ?? (
          <span className="text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell className={`text-right font-medium ${config.color}`}>
        {isNegative ? "-" : "+"}
        {formatCurrency(transaction.amount)}
        <YourShareText transaction={transaction} />
      </TableCell>
    </TableRow>
  );
}

function TransactionListItem({
  transaction,
  onClick,
}: {
  transaction: Transaction;
  onClick?: () => void;
}) {
  const config = TRANSACTION_TYPE_CONFIG[transaction.type];
  const Icon = config.icon;
  const isNegative =
    transaction.type === "expense" || transaction.type === "transfer";

  return (
    <div
      className="flex items-center justify-between py-3 px-3 -mx-3 rounded-md cursor-pointer hover:bg-accent/50 transition-colors"
      onClick={onClick}
    >
      <div className="flex items-center gap-3 min-w-0 flex-1">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-muted">
          <Icon className={`h-5 w-5 ${config.color}`} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium truncate">
            {transaction.description || config.label}
          </p>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{formatDate(transaction.date)}</span>
            <SharedKindBadge transaction={transaction} />
            {transaction.category && (
              <>
                <span>·</span>
                <span className="truncate">{transaction.category.name}</span>
              </>
            )}
          </div>
        </div>
      </div>
      <span className={`text-sm font-medium shrink-0 ml-3 text-right ${config.color}`}>
        {isNegative ? "-" : "+"}
        {formatCurrency(transaction.amount)}
        <YourShareText transaction={transaction} />
      </span>
    </div>
  );
}

export default function AccountDetailPage() {
  const params = useParams();
  const router = useRouter();
  const accountId = params.id as string;

  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<TransactionFilters>({});
  const [txDialogOpen, setTxDialogOpen] = useState(false);
  const [lendOpen, setLendOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editTxOpen, setEditTxOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null);
  const [addInvestmentOpen, setAddInvestmentOpen] = useState(false);
  const [investmentPage, setInvestmentPage] = useState(1);
  const [closeFDOpen, setCloseFDOpen] = useState(false);

  const { data: account, isLoading: accountLoading } = useAccount(accountId);
  const updateAccount = useUpdateAccount(accountId);
  const { hideBalances } = useAuth();
  const isInvestmentAccount = account?.type === "investment";
  const { data: transactionsData, isLoading: transactionsLoading } =
    useAccountTransactions(isInvestmentAccount ? "" : accountId, {
      ...filters,
      page,
      page_size: PAGE_SIZE,
    });
  const { data: categoriesData } = useCategories({ page_size: 100 });
  const { data: investmentsData, isLoading: investmentsLoading } =
    useAccountInvestments(
      isInvestmentAccount ? accountId : "",
      { page: investmentPage, page_size: PAGE_SIZE }
    );

  const transactions = transactionsData?.data ?? [];
  const totalPages = transactionsData?.total_pages ?? 1;
  const totalItems = transactionsData?.total_items ?? 0;
  const categories = [...(categoriesData?.data ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name)
  );
  const investments = investmentsData?.data ?? [];
  const investmentTotalPages = investmentsData?.total_pages ?? 1;
  const investmentTotalItems = investmentsData?.total_items ?? 0;

  if (accountLoading) {
    return <AccountDetailSkeleton />;
  }

  if (!account) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <h2 className="text-lg font-semibold">Account not found</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          This account may have been deleted or you don&apos;t have access.
        </p>
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => router.push("/accounts")}
        >
          Back to Accounts
        </Button>
      </div>
    );
  }

  const isFixedDepositAccount = account.type === "fixed_deposit";

  return (
    <div className="space-y-6">
      {/* Back navigation */}
      <Button asChild variant="ghost" size="sm">
        <Link href="/accounts">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Accounts
        </Link>
      </Button>

      {/* Account header */}
      <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <h1 className="text-2xl font-bold">{account.name}</h1>
            <Badge variant="secondary">
              {ACCOUNT_TYPE_LABELS[account.type] ?? account.type}
            </Badge>
            <Badge variant={account.is_active ? "outline" : "secondary"}>
              {account.is_active ? "Active" : "Inactive"}
            </Badge>
          </div>
          <p className="mt-2 text-3xl font-semibold">
            {formatCurrency(account.balance, account.currency, hideBalances)}
          </p>
          {account.description && (
            <p className="mt-1 text-sm text-muted-foreground">
              {account.description}
            </p>
          )}
          {isInvestmentAccount && (
            <div className="mt-2 flex gap-4 text-sm text-muted-foreground">
              {account.broker && <span>Broker: {account.broker}</span>}
              {account.account_number && (
                <span>Account: {account.account_number}</span>
              )}
            </div>
          )}
        </div>
        <div className="flex flex-col sm:flex-row gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={updateAccount.isPending}
            onClick={() => updateAccount.mutate({ is_pinned: !account.is_pinned })}
          >
            {updateAccount.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Pin className={`h-4 w-4 ${account.is_pinned ? "fill-primary" : ""}`} />
            )}
            <span className="ml-2">{account.is_pinned ? "Unpin" : "Pin"}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setEditDialogOpen(true)}
          >
            <Pencil className="h-4 w-4" />
            <span className="ml-2">Edit</span>
          </Button>
          {!isInvestmentAccount && !isFixedDepositAccount && (
            <Button size="sm" onClick={() => setTxDialogOpen(true)}>
              <Plus className="h-4 w-4" />
              <span className="ml-2">Add Transaction</span>
            </Button>
          )}
          {!isInvestmentAccount &&
            !isFixedDepositAccount &&
            isSettlementAccountType(account.type) && (
              <Button size="sm" variant="outline" onClick={() => setLendOpen(true)}>
                <Handshake className="h-4 w-4" />
                <span className="ml-2">Lend money</span>
              </Button>
            )}
          {isInvestmentAccount && (
            <Button size="sm" onClick={() => setAddInvestmentOpen(true)}>
              <Plus className="h-4 w-4" />
              <span className="ml-2">Add Investment</span>
            </Button>
          )}
          {isFixedDepositAccount && account.fixed_deposit && account.fixed_deposit.status !== "closed" && (
            <Button
              size="sm"
              variant={account.fixed_deposit.status === "matured" ? "default" : "outline"}
              onClick={() => setCloseFDOpen(true)}
            >
              <Vault className="h-4 w-4" />
              <span className="ml-2">
                {account.fixed_deposit.status === "matured" ? "Record payout" : "Close early"}
              </span>
            </Button>
          )}
        </div>
      </div>

      {/* Fixed Deposit details panel */}
      {isFixedDepositAccount && account.fixed_deposit && (
        <Card>
          <CardHeader>
            <CardTitle>Fixed Deposit Details</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <p className="text-sm text-muted-foreground">Principal</p>
                <p className="font-medium">{formatCurrency(account.fixed_deposit.principal, account.currency, hideBalances)}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Interest Rate</p>
                <p className="font-medium">{account.interest_rate ?? 0}% p.a.</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Start Date</p>
                <p className="font-medium">{formatDate(account.fixed_deposit.start_date)}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Maturity Date</p>
                <p className="font-medium">{formatDate(account.fixed_deposit.maturity_date)}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Payout Type</p>
                <p className="font-medium">{formatPayoutType(account.fixed_deposit.payout_type)}</p>
              </div>
              {account.fixed_deposit.payout_frequency && (
                <div>
                  <p className="text-sm text-muted-foreground">Payout Frequency</p>
                  <p className="font-medium">{formatPayoutFrequency(account.fixed_deposit.payout_frequency)}</p>
                </div>
              )}
              <div>
                <p className="text-sm text-muted-foreground">Compounding</p>
                <p className="font-medium">{formatCompounding(account.fixed_deposit.compounding)}</p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Projected Maturity Value</p>
                <p className="font-medium">{formatCurrency(account.fixed_deposit.maturity_value, account.currency, hideBalances)}</p>
              </div>
              <div className="sm:col-span-2">
                <p className="text-sm text-muted-foreground">Status</p>
                <Badge
                  variant={account.fixed_deposit.status === "matured" ? "default" : account.fixed_deposit.status === "closed" ? "secondary" : "outline"}
                  className={account.fixed_deposit.status === "matured" ? "bg-info text-info-foreground" : ""}
                >
                  {account.fixed_deposit.status === "matured" ? "Matured" : account.fixed_deposit.status === "closed" ? "Closed" : "Active"}
                </Badge>
              </div>
            </div>
            {/* Term progress bar */}
            {account.fixed_deposit.status !== "closed" && (() => {
              const start = new Date(account.fixed_deposit!.start_date.split("T")[0]).getTime();
              const end = new Date(account.fixed_deposit!.maturity_date.split("T")[0]).getTime();
              const progress = Math.min(Math.max(((Date.now() - start) / (end - start)) * 100, 0), 100);
              return (
                <div className="mt-4 space-y-1.5">
                  <div className="flex justify-between text-xs text-muted-foreground">
                    <span>Term progress</span>
                    <span>{progress.toFixed(0)}%</span>
                  </div>
                  <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                    <div className="h-full rounded-full bg-info" style={{ width: `${Math.max(progress, 2)}%` }} />
                  </div>
                </div>
              );
            })()}
          </CardContent>
        </Card>
      )}

      {/* Transactions section — hidden for investment accounts */}
      {!isInvestmentAccount && (
        <Card>
          <CardHeader>
            <CardTitle>Transactions</CardTitle>
            {totalItems > 0 && (
              <CardDescription>
                {totalItems} transaction{totalItems !== 1 ? "s" : ""}
              </CardDescription>
            )}
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Filters */}
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-1">
                <Label className="text-xs">From Date</Label>
                <Input
                  type="date"
                  value={filters.from_date ?? ""}
                  onChange={(e) => {
                    setFilters((f) => ({
                      ...f,
                      from_date: e.target.value || undefined,
                    }));
                    setPage(1);
                  }}
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">To Date</Label>
                <Input
                  type="date"
                  value={filters.to_date ?? ""}
                  onChange={(e) => {
                    setFilters((f) => ({
                      ...f,
                      to_date: e.target.value || undefined,
                    }));
                    setPage(1);
                  }}
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Type</Label>
                <Select
                  value={filters.type ?? "all"}
                  onValueChange={(val) => {
                    setFilters((f) => ({
                      ...f,
                      type:
                        val === "all"
                          ? undefined
                          : (val as TransactionType),
                    }));
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Types</SelectItem>
                    <SelectItem value="income">Income</SelectItem>
                    <SelectItem value="expense">Expense</SelectItem>
                    <SelectItem value="transfer">Transfer</SelectItem>
                    <SelectItem value="reimbursement">Repayment</SelectItem>
                    <SelectItem value="investment">Investment</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Category</Label>
                <Select
                  value={
                    filters.category_id ?? "all"
                  }
                  onValueChange={(val) => {
                    setFilters((f) => ({
                      ...f,
                      category_id:
                        val === "all" ? undefined : val,
                    }));
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Categories</SelectItem>
                    {categories.map((cat) => (
                      <SelectItem key={cat.id} value={cat.id}>
                        {cat.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Transactions table/list */}
            {transactionsLoading ? (
              <>
                {/* Mobile: List skeletons */}
                <div className="md:hidden space-y-3">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <Skeleton key={i} className="h-20 w-full" />
                  ))}
                </div>
                {/* Desktop: Table skeletons */}
                <div className="hidden md:block space-y-3">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <Skeleton key={i} className="h-12 w-full" />
                  ))}
                </div>
              </>
            ) : transactions.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
                <h3 className="text-lg font-semibold">No transactions yet</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  {Object.values(filters).some((v) => v !== undefined)
                    ? "No transactions match your filters. Try adjusting them."
                    : "Add your first transaction to get started."}
                </p>
              </div>
            ) : (
              <>
                {/* Mobile: List view */}
                <div className="md:hidden">
                  <div className="divide-y">
                    {transactions.map((tx) => (
                      <TransactionListItem
                        key={tx.id}
                        transaction={tx}
                        onClick={() => {
                          setSelectedTransaction(tx);
                          setEditTxOpen(true);
                        }}
                      />
                    ))}
                  </div>
                </div>

                {/* Desktop: Table view */}
                <div className="hidden md:block">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Date</TableHead>
                        <TableHead>Description</TableHead>
                        <TableHead>Type</TableHead>
                        <TableHead>Category</TableHead>
                        <TableHead className="text-right">Amount</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {transactions.map((tx) => (
                        <TransactionsTableRow
                          key={tx.id}
                          transaction={tx}
                          onClick={() => {
                            setSelectedTransaction(tx);
                            setEditTxOpen(true);
                          }}
                        />
                      ))}
                    </TableBody>
                  </Table>
                </div>

                {/* Pagination */}
                {totalPages > 1 && (
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">
                      Page {page} of {totalPages}
                    </span>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={page <= 1}
                        onClick={() => setPage((p) => p - 1)}
                      >
                        <ChevronLeft className="h-4 w-4" />
                        <span className="ml-1 hidden sm:inline">Previous</span>
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={page >= totalPages}
                        onClick={() => setPage((p) => p + 1)}
                      >
                        <span className="mr-1 hidden sm:inline">Next</span>
                        <ChevronRight className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>
      )}

      {/* Investment holdings */}
      {isInvestmentAccount && (
        <Card>
          <CardHeader>
            <CardTitle>Investments</CardTitle>
            {investmentTotalItems > 0 && (
              <CardDescription>
                {investmentTotalItems} holding
                {investmentTotalItems !== 1 ? "s" : ""}
              </CardDescription>
            )}
          </CardHeader>
          <CardContent>
            {investmentsLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : investments.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
                <h3 className="text-lg font-semibold">No investments yet</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  Add your first investment to start tracking your portfolio.
                </p>
              </div>
            ) : (
              <>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Symbol</TableHead>
                      <TableHead>Name</TableHead>
                      <TableHead className="text-right">Qty</TableHead>
                      <TableHead className="text-right">Price</TableHead>
                      <TableHead className="text-right">
                        Market Value
                      </TableHead>
                      <TableHead className="text-right">Gain/Loss</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {investments.map((inv) => {
                      const marketValue = Math.round(
                        inv.quantity * inv.current_price
                      );
                      const gainLoss = marketValue - inv.cost_basis;
                      const isPositive = gainLoss >= 0;
                      return (
                        <TableRow
                          key={inv.id}
                          className="cursor-pointer"
                          onClick={() =>
                            router.push(`/investments/${inv.id}`)
                          }
                        >
                          <TableCell className="font-mono font-semibold">
                            {inv.security.symbol}
                          </TableCell>
                          <TableCell>{inv.security.name}</TableCell>
                          <TableCell className="text-right">
                            {inv.quantity}
                          </TableCell>
                          <TableCell className="text-right">
                            {formatCurrency(inv.current_price, undefined, hideBalances)}
                          </TableCell>
                          <TableCell className="text-right font-medium">
                            {formatCurrency(marketValue, undefined, hideBalances)}
                          </TableCell>
                          <TableCell
                            className={`text-right font-medium ${
                              isPositive
                                ? "text-green-600"
                                : "text-red-600"
                            }`}
                          >
                            {isPositive ? "+" : ""}
                            {formatCurrency(gainLoss, undefined, hideBalances)}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>

                {investmentTotalPages > 1 && (
                  <div className="mt-4 flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">
                      Page {investmentPage} of {investmentTotalPages}
                    </span>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={investmentPage <= 1}
                        onClick={() =>
                          setInvestmentPage((p) => p - 1)
                        }
                      >
                        <ChevronLeft className="h-4 w-4" />
                        Previous
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={investmentPage >= investmentTotalPages}
                        onClick={() =>
                          setInvestmentPage((p) => p + 1)
                        }
                      >
                        Next
                        <ChevronRight className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>
      )}

      {!isInvestmentAccount && !isFixedDepositAccount && (
        <>
          <CreateTransactionDialog
            open={txDialogOpen}
            onOpenChange={setTxDialogOpen}
            defaultAccountId={accountId}
          />
          <LendMoneyDialog
            open={lendOpen}
            onOpenChange={setLendOpen}
            defaultAccountId={accountId}
          />
          <EditTransactionDialog
            open={editTxOpen}
            onOpenChange={setEditTxOpen}
            transaction={selectedTransaction}
          />
        </>
      )}

      <EditAccountDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        account={account}
      />

      {isInvestmentAccount && (
        <AddInvestmentDialog
          accountId={accountId}
          open={addInvestmentOpen}
          onOpenChange={setAddInvestmentOpen}
        />
      )}

      {isFixedDepositAccount && account.fixed_deposit && (
        <CloseFixedDepositDialog
          account={account}
          open={closeFDOpen}
          onOpenChange={setCloseFDOpen}
          onClosed={() => router.push("/accounts")}
        />
      )}
    </div>
  );
}
