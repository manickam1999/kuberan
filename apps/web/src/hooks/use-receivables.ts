import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Receivable } from "@/types/models";
import type {
  CounterpartiesResponse,
  CreateSharedExpenseRequest,
  PageResponse,
  ReceivableFilters,
  ReceivableResponse,
  ReceivablesSummary,
  RenameCounterpartyRequest,
  RenameCounterpartyResponse,
  SettleReceivableRequest,
  SettleReceivableResponse,
  TransactionResponse,
  UpdateReceivablesRequest,
  WriteOffReceivableRequest,
} from "@/types/api";
import { accountKeys } from "./use-accounts";
import { budgetKeys } from "./use-budgets";
import { transactionKeys } from "./use-transactions";

export const receivableKeys = {
  all: ["receivables"] as const,
  lists: () => [...receivableKeys.all, "list"] as const,
  list: (filters?: ReceivableFilters) =>
    [...receivableKeys.lists(), filters] as const,
  summary: () => [...receivableKeys.all, "summary"] as const,
  details: () => [...receivableKeys.all, "detail"] as const,
  detail: (id: string) => [...receivableKeys.details(), id] as const,
  counterparties: (q: string) =>
    [...receivableKeys.all, "counterparties", q] as const,
};

/**
 * Receivable mutations change balances, spending (personal_amount) and budget
 * progress, so refresh everything derived from them. Transaction keys cover the
 * analytics/summary/spending queries (they live under transactionKeys.all).
 * Net worth is intentionally unaffected by receivables (D4), so snapshots are
 * not invalidated.
 */
function invalidateReceivableDependents(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: receivableKeys.all });
  queryClient.invalidateQueries({ queryKey: transactionKeys.all });
  queryClient.invalidateQueries({ queryKey: accountKeys.all });
  queryClient.invalidateQueries({ queryKey: budgetKeys.allProgress() });
}

/** Paginated receivables, optionally filtered by status / kind / counterparty key. */
export function useReceivables(filters?: ReceivableFilters, enabled = true) {
  return useQuery({
    queryKey: receivableKeys.list(filters),
    queryFn: () =>
      apiClient.get<PageResponse<Receivable>>("/api/v1/receivables", {
        ...filters,
      }),
    enabled,
  });
}

const ALL_PAGE_SIZE = 100;

/**
 * Every receivable matching the filters, fetched page by page, so grouping and
 * totals computed from it are complete. Ignores any `page` / `page_size` in
 * `filters`.
 */
export function useAllReceivables(filters?: ReceivableFilters, enabled = true) {
  return useQuery({
    queryKey: [...receivableKeys.lists(), "all", filters] as const,
    queryFn: async () => {
      const all: Receivable[] = [];
      let page = 1;
      let totalPages = 1;
      do {
        const res = await apiClient.get<PageResponse<Receivable>>(
          "/api/v1/receivables",
          { ...filters, page, page_size: ALL_PAGE_SIZE }
        );
        all.push(...res.data);
        totalPages = res.total_pages;
        page += 1;
      } while (page <= totalPages);
      return all;
    },
    enabled,
  });
}

/** Totals of open receivables, grouped by kind and by counterparty. */
export function useReceivablesSummary() {
  return useQuery({
    queryKey: receivableKeys.summary(),
    queryFn: () =>
      apiClient.get<ReceivablesSummary>("/api/v1/receivables/summary"),
  });
}

/** One receivable including its settlements and expense. */
export function useReceivable(id: string, enabled = true) {
  return useQuery({
    queryKey: receivableKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<ReceivableResponse>(
        `/api/v1/receivables/${id}`
      );
      return res.receivable;
    },
    enabled: !!id && enabled,
  });
}

/** Counterparty autocomplete (max 20, most recently used first). */
export function useCounterparties(q: string, enabled = true) {
  return useQuery({
    queryKey: receivableKeys.counterparties(q),
    queryFn: async () => {
      const res = await apiClient.get<CounterpartiesResponse>(
        "/api/v1/receivables/counterparties",
        { q }
      );
      return res.counterparties;
    },
    enabled,
    staleTime: 30_000,
  });
}

/*
 * Toast convention: NO receivable mutation hook toasts. Callers own success and
 * error copy (use `receivableApiErrorMessage` from lib/receivable-format).
 */

/** POST /transactions/shared. */
export function useCreateSharedExpense() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateSharedExpenseRequest) => {
      const res = await apiClient.post<TransactionResponse>(
        "/api/v1/transactions/shared",
        data
      );
      return res.transaction;
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** PUT /transactions/:id/receivables (diff payload). */
export function useUpdateReceivables(transactionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: UpdateReceivablesRequest) => {
      const res = await apiClient.put<TransactionResponse>(
        `/api/v1/transactions/${transactionId}/receivables`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** Record a repayment against a receivable. */
export function useSettleReceivable() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      ...body
    }: SettleReceivableRequest & { id: string }) => {
      return apiClient.post<SettleReceivableResponse>(
        `/api/v1/receivables/${id}/settle`,
        body
      );
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** Reverse a repayment (soft-deletes its reimbursement transaction). */
export function useDeleteSettlement() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (settlementId: string) => {
      const res = await apiClient.del<ReceivableResponse>(
        `/api/v1/receivables/settlements/${settlementId}`
      );
      return res.receivable;
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** Forgive part (or all) of the outstanding balance; it becomes your spending. */
export function useWriteOff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      ...body
    }: WriteOffReceivableRequest & { id: string }) => {
      const res = await apiClient.post<ReceivableResponse>(
        `/api/v1/receivables/${id}/write-off`,
        body
      );
      return res.receivable;
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** Reset a receivable's written-off amount to 0. */
export function useUndoWriteOff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const res = await apiClient.del<ReceivableResponse>(
        `/api/v1/receivables/${id}/write-off`
      );
      return res.receivable;
    },
    onSuccess: () => invalidateReceivableDependents(queryClient),
  });
}

/** Rename every receivable of a counterparty; renaming onto an existing key merges. */
export function useRenameCounterparty() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: RenameCounterpartyRequest) => {
      return apiClient.post<RenameCounterpartyResponse>(
        "/api/v1/receivables/counterparties/rename",
        data
      );
    },
    onSuccess: () => {
      // Receivables are embedded in transaction lists, so refresh those too.
      queryClient.invalidateQueries({ queryKey: receivableKeys.all });
      queryClient.invalidateQueries({ queryKey: transactionKeys.all });
    },
  });
}

