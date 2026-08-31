"use client";

import { useEffect, useState } from "react";
import { useTopExpenses, useTransaction } from "@/hooks/use-transactions";
import { useCategories } from "@/hooks/use-categories";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatCurrency, formatDate } from "@/lib/format";

const LIMIT_OPTIONS = [10, 20, 50];

export function TopExpensesList({
  fromDate,
  toDate,
  hideBalances,
}: {
  fromDate: string;
  toDate: string;
  hideBalances: boolean;
}) {
  const [limit, setLimit] = useState(10);
  const [categoryId, setCategoryId] = useState("all");
  const { data: categoriesData } = useCategories({
    type: "expense",
    page_size: 100,
  });
  const categories = [...(categoriesData?.data ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name)
  );
  const { data, isLoading } = useTopExpenses(
    fromDate,
    toDate,
    limit,
    categoryId !== "all" ? categoryId : undefined
  );

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const { data: selectedTransaction } = useTransaction(selectedId ?? "");

  useEffect(() => {
    if (selectedTransaction && selectedTransaction.id === selectedId) {
      setEditOpen(true);
    }
  }, [selectedTransaction, selectedId]);

  const items = data?.items ?? [];

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3 space-y-0">
        <div>
          <CardTitle className="text-base">Top expenses</CardTitle>
          <CardDescription>
            Largest individual transactions this month
          </CardDescription>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Select value={categoryId} onValueChange={setCategoryId}>
            <SelectTrigger className="w-auto min-w-[9rem]">
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
          <Select
            value={String(limit)}
            onValueChange={(v) => setLimit(Number(v))}
          >
            <SelectTrigger className="w-20">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LIMIT_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">
            No expenses recorded
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8">#</TableHead>
                <TableHead>Description</TableHead>
                <TableHead>Category</TableHead>
                <TableHead>Account</TableHead>
                <TableHead>Date</TableHead>
                <TableHead className="text-right">Amount</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item, index) => (
                <TableRow
                  key={item.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedId(item.id)}
                >
                  <TableCell className="text-muted-foreground">
                    {index + 1}
                  </TableCell>
                  <TableCell className="font-medium">
                    {item.description || "—"}
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-1.5">
                      <span
                        className="flex size-5 shrink-0 items-center justify-center rounded text-[10px]"
                        style={{
                          backgroundColor: `${item.category_color}22`,
                          color: item.category_color,
                        }}
                      >
                        {item.category_icon ||
                          item.category_name.charAt(0).toUpperCase()}
                      </span>
                      <span className="truncate text-sm">
                        {item.category_name}
                      </span>
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {item.account_name}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(item.date)}
                  </TableCell>
                  <TableCell className="money text-right font-semibold text-negative">
                    {formatCurrency(item.amount, undefined, hideBalances)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <EditTransactionDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        transaction={selectedTransaction ?? null}
      />
    </Card>
  );
}
