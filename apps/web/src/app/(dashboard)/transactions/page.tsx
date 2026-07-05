"use client";

import { useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus, X } from "lucide-react";

import { useAccounts } from "@/hooks/use-accounts";
import { useTransactions } from "@/hooks/use-transactions";
import { useCategories } from "@/hooks/use-categories";
import { formatCurrency, formatDate } from "@/lib/format";
import { transactionVisual } from "@/lib/domain-visuals";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import type { Transaction, TransactionType } from "@/types/models";
import type { UserTransactionFilters } from "@/types/api";

const PAGE_SIZE = 25;

function TransactionRow({
  transaction,
  accountName,
  onClick,
}: {
  transaction: Transaction;
  accountName: string;
  onClick: () => void;
}) {
  const v = transactionVisual(transaction.type);
  const Icon = v.icon;

  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40"
    >
      <span
        className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${v.chip}`}
      >
        <Icon className="size-4" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {transaction.description || v.label}
        </p>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="truncate">{accountName}</span>
          {transaction.category && (
            <>
              <span>·</span>
              <span className="flex items-center gap-1 truncate">
                {transaction.category.color && (
                  <span
                    className="size-2 shrink-0 rounded-full"
                    style={{ backgroundColor: transaction.category.color }}
                  />
                )}
                {transaction.category.name}
              </span>
            </>
          )}
        </div>
      </div>
      <span className={`money shrink-0 text-sm font-semibold ${v.text}`}>
        {v.sign}
        {formatCurrency(transaction.amount)}
      </span>
    </button>
  );
}

export default function TransactionsPage() {
  const [page, setPage] = useState(1);
  const [accountFilter, setAccountFilter] = useState("all");
  const [typeFilter, setTypeFilter] = useState("all");
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [txDialogOpen, setTxDialogOpen] = useState(false);
  const [editTxOpen, setEditTxOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] =
    useState<Transaction | null>(null);

  const filters: UserTransactionFilters = {
    page,
    page_size: PAGE_SIZE,
    account_id: accountFilter !== "all" ? accountFilter : undefined,
    type: typeFilter !== "all" ? (typeFilter as TransactionType) : undefined,
    category_id: categoryFilter !== "all" ? categoryFilter : undefined,
    from_date: fromDate || undefined,
    to_date: toDate || undefined,
  };

  const { data, isLoading } = useTransactions(filters);
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const { data: categoriesData } = useCategories({ page_size: 100 });

  const transactions = useMemo(() => data?.data ?? [], [data?.data]);
  const totalPages = data?.total_pages ?? 1;
  const accounts = accountsData?.data ?? [];
  const categories = categoriesData?.data ?? [];

  const accountNameMap = new Map(accounts.map((a) => [a.id, a.name]));

  // Group transactions by day for readable scanning.
  const groups = useMemo(() => {
    const map = new Map<string, Transaction[]>();
    for (const tx of transactions) {
      const key = formatDate(tx.date);
      const arr = map.get(key);
      if (arr) arr.push(tx);
      else map.set(key, [tx]);
    }
    return Array.from(map.entries()).map(([date, txs]) => {
      const net = txs.reduce(
        (s, t) =>
          t.type === "income" ? s + t.amount : t.type === "expense" ? s - t.amount : s,
        0
      );
      return { date, txs, net };
    });
  }, [transactions]);

  const activeFilterCount = [
    accountFilter !== "all",
    typeFilter !== "all",
    categoryFilter !== "all",
    fromDate !== "",
    toDate !== "",
  ].filter(Boolean).length;

  const resetPage = () => setPage(1);
  const clearFilters = () => {
    setAccountFilter("all");
    setTypeFilter("all");
    setCategoryFilter("all");
    setFromDate("");
    setToDate("");
    resetPage();
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Transactions</h1>
          <p className="text-sm text-muted-foreground">
            {data?.total_items ?? transactions.length} total
          </p>
        </div>
        <Button size="sm" onClick={() => setTxDialogOpen(true)}>
          <Plus className="size-4" />
          Add transaction
        </Button>
      </div>

      {/* Filter toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={accountFilter}
          onValueChange={(v) => {
            setAccountFilter(v);
            resetPage();
          }}
        >
          <SelectTrigger className="h-9 w-auto min-w-[9rem]">
            <SelectValue placeholder="Account" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All accounts</SelectItem>
            {accounts.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={typeFilter}
          onValueChange={(v) => {
            setTypeFilter(v);
            resetPage();
          }}
        >
          <SelectTrigger className="h-9 w-auto min-w-[7.5rem]">
            <SelectValue placeholder="Type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All types</SelectItem>
            <SelectItem value="income">Income</SelectItem>
            <SelectItem value="expense">Expense</SelectItem>
            <SelectItem value="transfer">Transfer</SelectItem>
            <SelectItem value="investment">Investment</SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={categoryFilter}
          onValueChange={(v) => {
            setCategoryFilter(v);
            resetPage();
          }}
        >
          <SelectTrigger className="h-9 w-auto min-w-[9rem]">
            <SelectValue placeholder="Category" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All categories</SelectItem>
            {categories.map((cat) => (
              <SelectItem key={cat.id} value={cat.id}>
                {cat.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Input
          type="date"
          aria-label="From date"
          value={fromDate}
          onChange={(e) => {
            setFromDate(e.target.value);
            resetPage();
          }}
          className="h-9 w-auto"
        />
        <span className="text-sm text-muted-foreground">–</span>
        <Input
          type="date"
          aria-label="To date"
          value={toDate}
          onChange={(e) => {
            setToDate(e.target.value);
            resetPage();
          }}
          className="h-9 w-auto"
        />

        {activeFilterCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearFilters}
            className="h-9 text-muted-foreground"
          >
            <X className="size-4" />
            Clear
          </Button>
        )}
      </div>

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : transactions.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No transactions found</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {activeFilterCount > 0
              ? "No transactions match your filters."
              : "Add your first transaction to get started."}
          </p>
          {activeFilterCount > 0 ? (
            <Button className="mt-4" size="sm" variant="outline" onClick={clearFilters}>
              Clear filters
            </Button>
          ) : (
            <Button className="mt-4" size="sm" onClick={() => setTxDialogOpen(true)}>
              <Plus className="size-4" />
              Add transaction
            </Button>
          )}
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          {groups.map((group) => (
            <div key={group.date}>
              <div className="flex items-center justify-between bg-muted/40 px-4 py-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {group.date}
                </span>
                {group.net !== 0 && (
                  <span
                    className={`money text-xs font-medium ${
                      group.net > 0 ? "text-positive" : "text-negative"
                    }`}
                  >
                    {group.net > 0 ? "+" : "−"}
                    {formatCurrency(Math.abs(group.net))}
                  </span>
                )}
              </div>
              <div className="divide-y divide-border/50">
                {group.txs.map((tx) => (
                  <TransactionRow
                    key={tx.id}
                    transaction={tx}
                    accountName={
                      accountNameMap.get(tx.account_id) ?? "Unknown account"
                    }
                    onClick={() => {
                      setSelectedTransaction(tx);
                      setEditTxOpen(true);
                    }}
                  />
                ))}
              </div>
            </div>
          ))}
        </Card>
      )}

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
              <ChevronLeft className="size-4" />
              <span className="ml-1 hidden sm:inline">Previous</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              <span className="mr-1 hidden sm:inline">Next</span>
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}

      <CreateTransactionDialog open={txDialogOpen} onOpenChange={setTxDialogOpen} />
      <EditTransactionDialog
        open={editTxOpen}
        onOpenChange={setEditTxOpen}
        transaction={selectedTransaction}
      />
    </div>
  );
}
