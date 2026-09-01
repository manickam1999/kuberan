import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { localDayStartUTC, localDayEndUTC } from "@/lib/format";
import type { Transaction } from "@/types/models";
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
  });
}

export function useTransactions(filters?: UserTransactionFilters) {
  return useQuery({
    queryKey: transactionKeys.userList(filters),
    queryFn: () =>
      apiClient.get<PageResponse<Transaction>>("/api/v1/transactions", {
        ...filters,
      }),
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

export function useCreateTransaction() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateTransactionRequest) => {
      const res = await apiClient.post<TransactionResponse>(
        "/api/v1/transactions",
        data
      );
      return res.transaction;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: transactionKeys.all });
      queryClient.invalidateQueries({
        queryKey: accountKeys.detail(variables.account_id),
      });
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
      // Spending against a category budget changes; refresh budget progress
      // (budgets page stats/cards + dashboard budgets card).
      queryClient.invalidateQueries({ queryKey: budgetKeys.allProgress() });
    },
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
  return useMutation({
    mutationFn: async (data: UpdateTransactionRequest) => {
      const res = await apiClient.put<TransactionResponse>(
        `/api/v1/transactions/${id}`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: transactionKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.all });
      queryClient.invalidateQueries({ queryKey: budgetKeys.allProgress() });
    },
  });
}

export function useDeleteTransaction() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      await apiClient.del<DeleteResponse>(`/api/v1/transactions/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: transactionKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.all });
      queryClient.invalidateQueries({ queryKey: budgetKeys.allProgress() });
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
