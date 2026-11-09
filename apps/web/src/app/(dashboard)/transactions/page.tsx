"use client";

import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { ChevronLeft, ChevronRight, ListChecks, Plus, Search } from "lucide-react";

import { useAccounts, useAccountCurrencies } from "@/hooks/use-accounts";
import { useCategories } from "@/hooks/use-categories";
import { useTransactions } from "@/hooks/use-transactions";
import { shouldIgnoreShortcut } from "@/hooks/use-shell-shortcuts";
import {
  activeFilterCount,
  groupByDay,
  parseTxQuery,
  serializeTxQuery,
  toFilters,
  EMPTY_TX_QUERY,
  type TxQueryState,
} from "@/lib/transaction-list";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { EditTransactionDialog } from "@/components/transactions/edit-transaction-dialog";
import { flattenCategories } from "@/components/transactions/category-picker";
import { TransactionsFilterBar } from "@/components/transactions/transactions-filter-bar";
import {
  TransactionsList,
  TransactionsListSkeleton,
} from "@/components/transactions/transactions-list";
import { TransactionsBulkBar } from "@/components/transactions/transactions-bulk-bar";
import type { Transaction } from "@/types/models";

const PAGE_SIZE = 50;

function TransactionsView() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const state = useMemo(
    () => parseTxQuery(new URLSearchParams(searchParams.toString())),
    [searchParams]
  );

  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [pickerId, setPickerId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const update = useCallback(
    (patch: Partial<TxQueryState>) => {
      const next = { ...state, ...patch };
      const qs = serializeTxQuery(next);
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
      setSelected(new Set());
      setSelectMode(false);
      setActiveId(null);
    },
    [state, router, pathname]
  );

  const filters = useMemo(() => toFilters(state, PAGE_SIZE), [state]);
  const { data, isLoading, isError, refetch } = useTransactions(filters);
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const { data: categoriesData } = useCategories({ page_size: 100 });
  const currencyOf = useAccountCurrencies();

  const transactions = useMemo(() => data?.data ?? [], [data?.data]);
  const accounts = useMemo(() => accountsData?.data ?? [], [accountsData?.data]);
  const categories = useMemo(
    () => flattenCategories(categoriesData?.data ?? []),
    [categoriesData?.data]
  );
  const accountNames = useMemo(
    () => new Map(accounts.map((a) => [a.id, a.name])),
    [accounts]
  );
  const groups = useMemo(() => groupByDay(transactions), [transactions]);
  const total = data?.total_items ?? transactions.length;
  const totalPages = data?.total_pages ?? 1;
  const filterCount = activeFilterCount(state);
  const hasQuery = filterCount > 0 || state.q.trim() !== "";

  const selectedRows = useMemo(
    () => transactions.filter((t) => selected.has(t.id)),
    [transactions, selected]
  );
  // The detail dialog follows the live list row, so edits show immediately.
  const detailTx = useMemo(
    () => transactions.find((t) => t.id === detailId) ?? null,
    [transactions, detailId]
  );
  const detailRef = useRef<Transaction | null>(null);
  if (detailTx) detailRef.current = detailTx;

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);
  const toggleAll = useCallback((ids: string[], on: boolean) => {
    setSelected(on ? new Set(ids) : new Set());
  }, []);
  const clearSelection = useCallback(() => {
    setSelected(new Set());
    setSelectMode(false);
  }, []);
  const openDetail = useCallback((tx: Transaction) => {
    setActiveId(tx.id);
    setDetailId(tx.id);
  }, []);
  const enterSelectMode = useCallback((id: string) => {
    setSelectMode(true);
    setSelected(new Set([id]));
  }, []);

  // j/k move, x selects, c recategorises, e opens. Same guard as the shell
  // shortcuts: ignored while typing or while a dialog, menu or popover is open.
  const flat = useMemo(() => transactions.map((t) => t.id), [transactions]);
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (shouldIgnoreShortcut(e)) return;
      const key = e.key;
      if (key === "Escape") {
        if (selected.size > 0 || selectMode) {
          e.preventDefault();
          clearSelection();
        }
        return;
      }
      if (flat.length === 0) return;
      const index = activeId ? flat.indexOf(activeId) : -1;
      if (key === "j" || key === "k") {
        e.preventDefault();
        const next =
          key === "j"
            ? Math.min(flat.length - 1, index + 1)
            : Math.max(0, index === -1 ? 0 : index - 1);
        const id = flat[next];
        setActiveId(id);
        requestAnimationFrame(() =>
          document
            .querySelector(`[data-tx-id="${id}"]`)
            ?.scrollIntoView({ block: "nearest" })
        );
      } else if (activeId && index !== -1) {
        if (key === "x") {
          e.preventDefault();
          toggle(activeId);
        } else if (key === "c") {
          e.preventDefault();
          const tx = transactions[index];
          if (window.matchMedia("(min-width: 768px)").matches) {
            if (tx.type === "expense" || tx.type === "income") setPickerId(activeId);
          } else {
            openDetail(tx);
          }
        } else if (key === "e") {
          e.preventDefault();
          openDetail(transactions[index]);
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [flat, activeId, selected, selectMode, transactions, toggle, clearSelection, openDetail]);

  return (
    <div className={cn("space-y-4", selected.size > 0 && "max-md:pb-20")}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Transactions
          </h1>
          <p className="text-muted-foreground text-sm">
            {isLoading
              ? "Loading…"
              : `${total.toLocaleString("en-US")} ${total === 1 ? "transaction" : "transactions"}${hasQuery ? " match" : ""}`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant={selectMode ? "secondary" : "outline"}
            size="sm"
            className="md:hidden"
            data-select-toggle
            onClick={() => (selectMode ? clearSelection() : setSelectMode(true))}
          >
            <ListChecks />
            {selectMode ? "Done" : "Select"}
          </Button>
          <Button size="sm" className="max-md:hidden" onClick={() => setCreateOpen(true)}>
            <Plus />
            Add
          </Button>
        </div>
      </div>

      <TransactionsFilterBar
        state={state}
        onChange={update}
        accounts={accounts}
        categories={categories}
        searchRef={searchRef}
      />

      {selected.size > 0 && (
        <TransactionsBulkBar
          selected={selectedRows}
          categories={categories}
          currencyOf={(t) => currencyOf(t.account_id, t.account)}
          onClear={clearSelection}
        />
      )}

      {isError && !data ? (
        <div className="flex flex-col items-center rounded-card border border-dashed p-10 text-center">
          <h2 className="text-base font-semibold">Couldn&apos;t load transactions</h2>
          <p className="text-muted-foreground mt-1 text-sm">
            Check your connection and try again.
          </p>
          <Button className="mt-4" size="sm" variant="outline" onClick={() => refetch()}>
            Try again
          </Button>
        </div>
      ) : isLoading ? (
        <TransactionsListSkeleton />
      ) : transactions.length === 0 ? (
        <div className="flex flex-col items-center rounded-card border border-dashed p-10 text-center">
          <Search className="text-muted-foreground mb-2 size-6" aria-hidden />
          <h2 className="text-base font-semibold">
            {hasQuery ? "Nothing matches" : "No transactions yet"}
          </h2>
          <p className="text-muted-foreground mt-1 text-sm">
            {hasQuery
              ? "Try a different search or loosen the filters."
              : "Add your first transaction to get started."}
          </p>
          {hasQuery ? (
            <Button
              className="mt-4"
              size="sm"
              variant="outline"
              onClick={() => update({ ...EMPTY_TX_QUERY })}
            >
              Clear search and filters
            </Button>
          ) : (
            <Button className="mt-4" size="sm" onClick={() => setCreateOpen(true)}>
              <Plus />
              Add transaction
            </Button>
          )}
        </div>
      ) : (
        <TransactionsList
          groups={groups}
          categories={categories}
          accountName={(id) => accountNames.get(id) ?? "Unknown account"}
          currencyOf={(t) => currencyOf(t.account_id, t.account)}
          selected={selected}
          selectMode={selectMode || selected.size > 0}
          onEnterSelectMode={enterSelectMode}
          activeId={activeId}
          onToggle={toggle}
          onToggleAll={toggleAll}
          onOpen={openDetail}
          pickerId={pickerId}
          onPickerChange={setPickerId}
        />
      )}

      {transactions.length > 0 && (
        <p className="text-muted-foreground max-md:hidden text-xs">
          <kbd className="font-mono">j</kbd> <kbd className="font-mono">k</kbd> move ·{" "}
          <kbd className="font-mono">x</kbd> select · <kbd className="font-mono">c</kbd>{" "}
          category · <kbd className="font-mono">e</kbd> edit
        </p>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-muted-foreground text-sm">
            Page {state.page} of {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={state.page <= 1}
              onClick={() => update({ page: state.page - 1 })}
            >
              <ChevronLeft />
              <span className="hidden sm:inline">Previous</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={state.page >= totalPages}
              onClick={() => update({ page: state.page + 1 })}
            >
              <span className="hidden sm:inline">Next</span>
              <ChevronRight />
            </Button>
          </div>
        </div>
      )}

      <CreateTransactionDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditTransactionDialog
        open={detailId !== null}
        onOpenChange={(open) => {
          if (!open) setDetailId(null);
        }}
        transaction={detailTx ?? detailRef.current}
      />
    </div>
  );
}

export default function TransactionsPage() {
  return (
    <Suspense fallback={<TransactionsListSkeleton />}>
      <TransactionsView />
    </Suspense>
  );
}
