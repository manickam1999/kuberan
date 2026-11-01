import {
  keepPreviousData,
  useQuery,
  useMutation,
  useQueryClient,
  type QueryClient,
  type QueryKey,
} from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { localDayStartUTC, localDayEndUTC } from "@/lib/format";
import {
  optimisticId,
  patchInPage,
  removeFromPage,
  restoreSnapshot,
  snapshotQueries,
  updatePages,
  type OptimisticContext,
} from "@/lib/optimistic";
import type { Account, Category, Transaction } from "@/types/models";
import type {
  PageResponse,
  TransactionResponse,
  DeleteResponse,
  TransactionFilters,
  UserTransactionFilters,
  CreateTransactionRequest,
  CreateTransferRequest,
  UpdateTransactionRequest,
  SpendingByCategory,
  Cashflow,
  MonthlySummaryItem,
  DailySpendingItem,
  DailySummaryItem,
  TopExpenses,
} from "@/types/api";
import { accountKeys } from "./use-accounts";
import { budgetKeys } from "./use-budgets";
import { categoryKeys } from "./use-categories";
import { receivableKeys } from "./use-receivables";

export const transactionKeys = {
  all: ["transactions"] as const,
  lists: () => [...transactionKeys.all, "list"] as const,
  listByAccount: (accountId: string, filters?: TransactionFilters) =>
    [...transactionKeys.lists(), "account", accountId, filters] as const,
  userLists: () => [...transactionKeys.all, "userList"] as const,
  userList: (filters?: UserTransactionFilters) =>
    [...transactionKeys.userLists(), filters] as const,
  details: () => [...transactionKeys.all, "detail"] as const,
  detail: (id: string) => [...transactionKeys.details(), id] as const,
  spendingByCategory: (from: string, to: string) =>
    [...transactionKeys.all, "spendingByCategory", from, to] as const,
  cashflow: (from: string, to: string) =>
    [...transactionKeys.all, "cashflow", from, to] as const,
  monthlySummary: (months: number) =>
    [...transactionKeys.all, "monthlySummary", months] as const,
  dailySpending: (from: string, to: string) =>
    [...transactionKeys.all, "dailySpending", from, to] as const,
  dailySummary: (from: string, to: string) =>
    [...transactionKeys.all, "dailySummary", from, to] as const,
  topExpenses: (
    from: string,
    to: string,
    limit: number,
    categoryId?: string
  ) =>
    [...transactionKeys.all, "topExpenses", from, to, limit, categoryId] as const,
};

export function useAccountTransactions(
  accountId: string,
  filters?: TransactionFilters
) {
  return useQuery({
    queryKey: transactionKeys.listByAccount(accountId, filters),
    queryFn: () =>
      apiClient.get<PageResponse<Transaction>>(
        `/api/v1/accounts/${accountId}/transactions`,
        { ...filters }
      ),
    enabled: !!accountId,
    // Keep showing the current page while the next page / filter loads, but
    // never another account's rows when navigating between accounts.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[3] === accountId ? previous : undefined,
  });
}

export function useTransactions(filters?: UserTransactionFilters) {
  return useQuery({
    queryKey: transactionKeys.userList(filters),
    queryFn: () =>
      apiClient.get<PageResponse<Transaction>>("/api/v1/transactions", {
        ...filters,
      }),
    placeholderData: keepPreviousData,
  });
}

export function useTransaction(id: string) {
  return useQuery({
    queryKey: transactionKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<TransactionResponse>(
        `/api/v1/transactions/${id}`
      );
      return res.transaction;
    },
    enabled: !!id,
  });
}

// --- Optimistic cache helpers -------------------------------------------
//
// Transaction lists are paginated and filtered server-side, so an optimistic
// change can only approximate them: rows are patched or removed in place, and
// a new row is inserted only into first pages whose filters it provably
// matches. Every mutation still invalidates on settle, so the server's answer
// always wins.

const TRANSACTION_LIST_PREFIXES = [
  transactionKeys.lists(),
  transactionKeys.userLists(),
];

/** Filters stored in a list query key (see transactionKeys). */
function filtersFromKey(queryKey: QueryKey): UserTransactionFilters | undefined {
  const [, kind, scope, accountId, filters] = queryKey as readonly unknown[];
  if (kind === "list" && scope === "account" && typeof accountId === "string") {
    return {
      ...((filters ?? {}) as TransactionFilters),
      account_id: accountId,
    };
  }
  if (kind === "userList") {
    return (scope ?? undefined) as UserTransactionFilters | undefined;
  }
  return undefined;
}

/**
 * Whether a new transaction certainly belongs on the first page of a list
 * with these filters. Date-range filters are interpreted in the user's local
 * day by the server, so lists with them are left to the refetch.
 */
function belongsOnFirstPage(
  tx: Transaction,
  filters: UserTransactionFilters | undefined
): boolean {
  if (!filters) return true;
  if ((filters.page ?? 1) !== 1) return false;
  if (filters.from_date || filters.to_date) return false;
  if (filters.account_id && filters.account_id !== tx.account_id) return false;
  if (filters.type && filters.type !== tx.type) return false;
  if (filters.category_id && filters.category_id !== tx.category_id) return false;
  if (filters.min_amount !== undefined && tx.amount < filters.min_amount) return false;
  if (filters.max_amount !== undefined && tx.amount > filters.max_amount) return false;
  return true;
}

/** Insert keeping the server's "date DESC" order and the page size. */
function insertByDate(
  page: PageResponse<Transaction>,
  tx: Transaction
): PageResponse<Transaction> {
  const index = page.data.findIndex((existing) => existing.date <= tx.date);
  const data = [...page.data];
  data.splice(index === -1 ? data.length : index, 0, tx);
  if (page.page_size > 0 && data.length > page.page_size) data.pop();
  return { ...page, data, total_items: page.total_items + 1 };
}

function cachedAccount(queryClient: QueryClient, id: string): Account | undefined {
  const detail = queryClient.getQueryData<Account>(accountKeys.detail(id));
  if (detail) return detail;
  for (const [, page] of queryClient.getQueriesData<PageResponse<Account>>({
    queryKey: accountKeys.lists(),
  })) {
    const found = page?.data?.find((a) => a.id === id);
    if (found) return found;
  }
  return undefined;
}

function cachedCategory(queryClient: QueryClient, id: string): Category | undefined {
  for (const [, page] of queryClient.getQueriesData<PageResponse<Category>>({
    queryKey: categoryKeys.lists(),
  })) {
    for (const category of page?.data ?? []) {
      if (category.id === id) return category;
      const child = category.children?.find((c) => c.id === id);
      if (child) return child;
    }
  }
  return undefined;
}

/** Apply an update request to a cached transaction, resolving relations from cache. */
function applyTransactionUpdate(
  queryClient: QueryClient,
  tx: Transaction,
  data: UpdateTransactionRequest
): Transaction {
  const next: Transaction = { ...tx };
  if (data.type !== undefined) next.type = data.type;
  if (data.amount !== undefined) next.amount = data.amount;
  if (data.description !== undefined) next.description = data.description;
  if (data.date !== undefined) next.date = data.date;
  if (data.account_id !== undefined && data.account_id !== tx.account_id) {
    next.account_id = data.account_id;
    next.account = cachedAccount(queryClient, data.account_id);
  }
  if (data.category_id !== undefined && data.category_id !== tx.category_id) {
    next.category_id = data.category_id;
    next.category = data.category_id
      ? (cachedCategory(queryClient, data.category_id) ?? null)
      : null;
  }
  return next;
}

/** The query keys every transaction mutation refreshes once it settles. */
function invalidateAfterTransactionChange(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: transactionKeys.all });
  queryClient.invalidateQueries({ queryKey: accountKeys.all });
  // Spending against a category budget changes; refresh budget progress
  // (budgets page stats/cards + dashboard budgets card).
  queryClient.invalidateQueries({ queryKey: budgetKeys.allProgress() });
}

export function useCreateTransaction() {
  const queryClient = useQueryClient();
  return useMutation<
    Transaction,
    Error,
    CreateTransactionRequest,
    OptimisticContext
  >({
    mutationFn: async (data) => {
      const res = await apiClient.post<TransactionResponse>(
        "/api/v1/transactions",
        data
      );
      return res.transaction;
    },
    onMutate: async (data) => {
      const snapshot = await snapshotQueries(
        queryClient,
        TRANSACTION_LIST_PREFIXES
      );
      const now = new Date().toISOString();
      const optimistic: Transaction = {
        id: optimisticId(),
        created_at: now,
        updated_at: now,
        user_id: "",
        account_id: data.account_id,
        category_id: data.category_id ?? null,
        type: data.type,
        amount: data.amount,
        description: data.description ?? "",
        date: data.date ?? now,
        account: cachedAccount(queryClient, data.account_id),
        category: data.category_id
          ? (cachedCategory(queryClient, data.category_id) ?? null)
          : null,
        personal_amount: null,
      };
      for (const prefix of TRANSACTION_LIST_PREFIXES) {
        updatePages<Transaction>(queryClient, prefix, (page, queryKey) =>
          belongsOnFirstPage(optimistic, filtersFromKey(queryKey))
            ? insertByDate(page, optimistic)
            : page
        );
      }
      return { snapshot };
    },
    onError: (_err, _data, context) => restoreSnapshot(queryClient, context),
    onSettled: () => invalidateAfterTransactionChange(queryClient),
  });
}

export function useCreateTransfer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateTransferRequest) => {
      const res = await apiClient.post<TransactionResponse>(
        "/api/v1/transactions/transfer",
        data
      );
      return res.transaction;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: transactionKeys.all });
      queryClient.invalidateQueries({
        queryKey: accountKeys.detail(variables.from_account_id),
      });
      queryClient.invalidateQueries({
        queryKey: accountKeys.detail(variables.to_account_id),
      });
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
    },
  });
}

export function useUpdateTransaction(id: string) {
  const queryClient = useQueryClient();
  return useMutation<
    Transaction,
    Error,
    UpdateTransactionRequest,
    OptimisticContext
  >({
    mutationFn: async (data) => {
      const res = await apiClient.put<TransactionResponse>(
        `/api/v1/transactions/${id}`,
        data
      );
      return res.transaction;
    },
    onMutate: async (data) => {
      const snapshot = await snapshotQueries(queryClient, [
        ...TRANSACTION_LIST_PREFIXES,
        transactionKeys.detail(id),
      ]);
      for (const prefix of TRANSACTION_LIST_PREFIXES) {
        updatePages<Transaction>(queryClient, prefix, (page) =>
          patchInPage(page, id, (tx) =>
            applyTransactionUpdate(queryClient, tx, data)
          )
        );
      }
      queryClient.setQueryData<Transaction>(transactionKeys.detail(id), (tx) =>
        tx ? applyTransactionUpdate(queryClient, tx, data) : tx
      );
      return { snapshot };
    },
    onError: (_err, _data, context) => restoreSnapshot(queryClient, context),
    onSettled: () => {
      invalidateAfterTransactionChange(queryClient);
      // personal_amount / embedded receivables may have changed.
      queryClient.invalidateQueries({ queryKey: receivableKeys.all });
    },
  });
}

export function useDeleteTransaction() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string, OptimisticContext>({
    mutationFn: async (id) => {
      await apiClient.del<DeleteResponse>(`/api/v1/transactions/${id}`);
    },
    onMutate: async (id) => {
      const snapshot = await snapshotQueries(
        queryClient,
        TRANSACTION_LIST_PREFIXES
      );
      for (const prefix of TRANSACTION_LIST_PREFIXES) {
        updatePages<Transaction>(queryClient, prefix, (page) =>
          removeFromPage(page, id)
        );
      }
      return { snapshot };
    },
    onError: (_err, _id, context) => restoreSnapshot(queryClient, context),
    onSettled: (_data, err, id) => {
      // Don't refetch the detail of a transaction that no longer exists.
      if (!err) queryClient.removeQueries({ queryKey: transactionKeys.detail(id) });
      invalidateAfterTransactionChange(queryClient);
      // Deleting a shared expense cascades to its receivables.
      queryClient.invalidateQueries({ queryKey: receivableKeys.all });
    },
  });
}

export function useSpendingByCategory(from: string, to: string) {
  return useQuery({
    queryKey: transactionKeys.spendingByCategory(from, to),
    queryFn: () =>
      apiClient.get<SpendingByCategory>(
        "/api/v1/transactions/spending-by-category",
        { from_date: localDayStartUTC(from), to_date: localDayEndUTC(to) }
      ),
  });
}

export function useCashflow(from: string, to: string) {
  return useQuery({
    queryKey: transactionKeys.cashflow(from, to),
    queryFn: () =>
      apiClient.get<Cashflow>("/api/v1/transactions/cashflow", {
        from_date: localDayStartUTC(from),
        to_date: localDayEndUTC(to),
      }),
  });
}

export function useMonthlySummary(months = 6) {
  return useQuery({
    queryKey: transactionKeys.monthlySummary(months),
    queryFn: () =>
      apiClient.get<{ data: MonthlySummaryItem[] }>(
        "/api/v1/transactions/monthly-summary",
        { months }
      ),
    select: (res) => res.data,
  });
}

export function useDailySpending(from: string, to: string) {
  return useQuery({
    queryKey: transactionKeys.dailySpending(from, to),
    queryFn: () =>
      apiClient.get<{ data: DailySpendingItem[] }>(
        "/api/v1/transactions/daily-spending",
        { from_date: localDayStartUTC(from), to_date: localDayEndUTC(to) }
      ),
    select: (res) => res.data,
  });
}

export function useDailySummary(from: string, to: string) {
  return useQuery({
    queryKey: transactionKeys.dailySummary(from, to),
    queryFn: () =>
      apiClient.get<{ data: DailySummaryItem[] }>(
        "/api/v1/transactions/daily-summary",
        { from_date: localDayStartUTC(from), to_date: localDayEndUTC(to) }
      ),
    select: (res) => res.data,
  });
}

export function useTopExpenses(
  from: string,
  to: string,
  limit = 10,
  categoryId?: string
) {
  return useQuery({
    queryKey: transactionKeys.topExpenses(from, to, limit, categoryId),
    queryFn: () =>
      apiClient.get<TopExpenses>("/api/v1/transactions/top-expenses", {
        from_date: localDayStartUTC(from),
        to_date: localDayEndUTC(to),
        limit,
        category_id: categoryId,
      }),
  });
}
