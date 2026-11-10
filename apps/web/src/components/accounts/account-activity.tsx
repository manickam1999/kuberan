"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useAccountTransactions } from "@/hooks/use-transactions";
import { useCategories } from "@/hooks/use-categories";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { TransactionAmount } from "@/components/transactions/transaction-amount";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { isOutflow } from "@/lib/account-math";
import { transactionVisual } from "@/lib/domain-visuals";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction, TransactionType } from "@/types/models";
import type { TransactionFilters } from "@/types/api";

const PAGE_SIZE = 20;

/** Colour of an amount: income and repayments read positive, the rest neutral. */
function amountClass(t: Transaction, outflow: boolean): string {
  if (t.type === "opening_balance") return "text-muted-foreground";
  if (t.type === "income" || t.type === "reimbursement") return "text-positive";
  if (t.type === "transfer") return "text-info";
  return outflow ? "text-foreground" : "text-positive";
}

function ActivityRow({
  transaction,
  accountId,
  onOpen,
}: {
  transaction: Transaction;
  accountId: string;
  onOpen: () => void;
}) {
  const v = transactionVisual(transaction.type);
  const Icon = v.icon;
  const outflow = isOutflow(transaction, accountId);
  return (
    <TableRow className="cursor-pointer" onClick={onOpen}>
      <TableCell className="text-muted-foreground whitespace-nowrap">
        {formatDate(transaction.date)}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "grid size-6 place-items-center rounded-full",
              v.chip,
            )}
          >
            <Icon aria-hidden className="size-3" />
          </span>
          <button
            type="button"
            className="focus-visible:ring-ring/50 rounded-sm text-left outline-none focus-visible:ring-[3px]"
            onClick={(e) => {
              e.stopPropagation();
              onOpen();
            }}
          >
            {transaction.description || v.label}
          </button>
          <SharedKindBadge transaction={transaction} />
        </div>
      </TableCell>
      <TableCell>
        <Badge variant="outline" className={v.text}>
          {v.label}
        </Badge>
      </TableCell>
      <TableCell>
        {transaction.category?.name ?? (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell
        className={cn(
          "text-right font-medium",
          amountClass(transaction, outflow),
        )}
      >
        <TransactionAmount
          transaction={transaction}
          viewAccountId={accountId}
          direction={outflow ? "-" : "+"}
        />
        <YourShareText transaction={transaction} />
      </TableCell>
    </TableRow>
  );
}

function ActivityListItem({
  transaction,
  accountId,
  onOpen,
}: {
  transaction: Transaction;
  accountId: string;
  onOpen: () => void;
}) {
  const v = transactionVisual(transaction.type);
  const Icon = v.icon;
  const outflow = isOutflow(transaction, accountId);
  return (
    <button
      type="button"
      onClick={onOpen}
      className="hover:bg-accent/50 focus-visible:ring-ring/50 -mx-3 flex w-[calc(100%+1.5rem)] items-center justify-between rounded-md px-3 py-3 text-left outline-none transition-colors focus-visible:ring-[3px]"
    >
      <span className="flex min-w-0 flex-1 items-center gap-3">
        <span
          className={cn(
            "grid size-10 shrink-0 place-items-center rounded-full",
            v.chip,
          )}
        >
          <Icon aria-hidden className="size-5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {transaction.description || v.label}
          </span>
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
            <span className="whitespace-nowrap">
              {formatDate(transaction.date)}
            </span>
            <SharedKindBadge transaction={transaction} />
            {transaction.category && (
              <>
                <span>·</span>
                <span className="truncate">{transaction.category.name}</span>
              </>
            )}
          </span>
        </span>
      </span>
      <span
        className={cn(
          "ml-3 shrink-0 text-right text-sm font-medium",
          amountClass(transaction, outflow),
        )}
      >
        <TransactionAmount
          transaction={transaction}
          viewAccountId={accountId}
          direction={outflow ? "-" : "+"}
        />
        <YourShareText transaction={transaction} />
      </span>
    </button>
  );
}

/**
 * The account's transactions: filters, a table on desktop and a list on
 * phones, paged. Rows open the edit dialog.
 */
export function AccountActivity({ accountId }: { accountId: string }) {
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<TransactionFilters>({});
  const [selected, setSelected] = useState<Transaction | null>(null);
  const [editOpen, setEditOpen] = useState(false);

  const { data, isLoading, isError, refetch } = useAccountTransactions(
    accountId,
    {
      ...filters,
      page,
      page_size: PAGE_SIZE,
    },
  );
  const { data: categoriesData } = useCategories({ page_size: 100 });

  const transactions = data?.data ?? [];
  const totalPages = data?.total_pages ?? 1;
  const totalItems = data?.total_items ?? 0;
  const categories = [...(categoriesData?.data ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name),
  );
  const filtered = Object.values(filters).some((v) => v !== undefined);

  const update = (patch: Partial<TransactionFilters>) => {
    setFilters((f) => ({ ...f, ...patch }));
    setPage(1);
  };
  const open = (tx: Transaction) => {
    setSelected(tx);
    setEditOpen(true);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Activity</CardTitle>
        {totalItems > 0 && (
          <CardDescription>
            {totalItems} transaction{totalItems !== 1 ? "s" : ""}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-1">
            <Label htmlFor="act-from" className="text-xs">
              From date
            </Label>
            <Input
              id="act-from"
              type="date"
              value={filters.from_date ?? ""}
              onChange={(e) =>
                update({ from_date: e.target.value || undefined })
              }
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="act-to" className="text-xs">
              To date
            </Label>
            <Input
              id="act-to"
              type="date"
              value={filters.to_date ?? ""}
              onChange={(e) => update({ to_date: e.target.value || undefined })}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="act-type" className="text-xs">
              Type
            </Label>
            <Select
              value={filters.type ?? "all"}
              onValueChange={(val) =>
                update({
                  type: val === "all" ? undefined : (val as TransactionType),
                })
              }
            >
              <SelectTrigger id="act-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All types</SelectItem>
                <SelectItem value="income">Income</SelectItem>
                <SelectItem value="expense">Expense</SelectItem>
                <SelectItem value="transfer">Transfer</SelectItem>
                <SelectItem value="reimbursement">Repayment</SelectItem>
                <SelectItem value="opening_balance">Opening balance</SelectItem>
                <SelectItem value="investment">Investment</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label htmlFor="act-category" className="text-xs">
              Category
            </Label>
            <Select
              value={filters.category_id ?? "all"}
              onValueChange={(val) =>
                update({ category_id: val === "all" ? undefined : val })
              }
            >
              <SelectTrigger id="act-category">
                <SelectValue />
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
          </div>
        </div>

        {isLoading ? (
          <div
            className="space-y-3"
            aria-busy="true"
            aria-label="Loading activity"
          >
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full md:h-12" />
            ))}
          </div>
        ) : isError ? (
          <div
            role="alert"
            className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-center"
          >
            <p className="text-sm">Couldn&apos;t load transactions.</p>
            <Button size="sm" variant="outline" onClick={() => refetch()}>
              Try again
            </Button>
          </div>
        ) : transactions.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
            <h3 className="text-lg font-semibold">No transactions</h3>
            <p className="text-muted-foreground mt-1 text-sm">
              {filtered
                ? "Nothing matches these filters."
                : "Add the first transaction to see it here."}
            </p>
            {filtered && (
              <Button
                className="mt-3"
                size="sm"
                variant="outline"
                onClick={() => {
                  setFilters({});
                  setPage(1);
                }}
              >
                Clear filters
              </Button>
            )}
          </div>
        ) : (
          <>
            <div className="divide-y md:hidden">
              {transactions.map((tx) => (
                <ActivityListItem
                  key={tx.id}
                  transaction={tx}
                  accountId={accountId}
                  onOpen={() => open(tx)}
                />
              ))}
            </div>
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
                    <ActivityRow
                      key={tx.id}
                      transaction={tx}
                      accountId={accountId}
                      onOpen={() => open(tx)}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground text-sm">
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
          </>
        )}
      </CardContent>
      <EditTransactionDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        transaction={selected}
      />
    </Card>
  );
}
