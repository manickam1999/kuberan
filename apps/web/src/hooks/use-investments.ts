import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Investment, InvestmentTransaction } from "@/types/models";
import type {
  PageResponse,
  PaginationParams,
  InvestmentResponse,
  PortfolioResponse,
  InvestmentTransactionResponse,
  AddInvestmentRequest,
  RecordBuyRequest,
  RecordSellRequest,
  RecordDividendRequest,
  RecordSplitRequest,
} from "@/types/api";
import { accountKeys } from "./use-accounts";

export type InvestmentStatus = "open" | "closed" | "all";

export type AllInvestmentsParams = PaginationParams & {
  status?: InvestmentStatus;
};

export const investmentKeys = {
  all: ["investments"] as const,
  portfolio: () => [...investmentKeys.all, "portfolio"] as const,
  allList: (params?: AllInvestmentsParams) =>
    [...investmentKeys.all, "all", params] as const,
  lists: () => [...investmentKeys.all, "list"] as const,
  list: (accountId: string, params?: PaginationParams) =>
    [...investmentKeys.lists(), accountId, params] as const,
  details: () => [...investmentKeys.all, "detail"] as const,
  detail: (id: string) => [...investmentKeys.details(), id] as const,
  transactions: (id: string, params?: PaginationParams) =>
    [...investmentKeys.all, "transactions", id, params] as const,
};

export function usePortfolio() {
  return useQuery({
    queryKey: investmentKeys.portfolio(),
    queryFn: async () => {
      const res = await apiClient.get<PortfolioResponse>(
        "/api/v1/investments/portfolio"
      );
      return res.portfolio;
    },
  });
}

export function useAllInvestments(params?: AllInvestmentsParams) {
  return useQuery({
    queryKey: investmentKeys.allList(params),
    queryFn: () =>
      apiClient.get<PageResponse<Investment>>("/api/v1/investments", {
        ...params,
      }),
  });
}

export function useInvestment(id: string) {
  return useQuery({
    queryKey: investmentKeys.detail(id),
    queryFn: async () => {
      const res = await apiClient.get<InvestmentResponse>(
        `/api/v1/investments/${id}`
      );
      return res.investment;
    },
    enabled: !!id,
  });
}

export function useAccountInvestments(
  accountId: string,
  params?: PaginationParams
) {
  return useQuery({
    queryKey: investmentKeys.list(accountId, params),
    queryFn: () =>
      apiClient.get<PageResponse<Investment>>(
        `/api/v1/accounts/${accountId}/investments`,
        { ...params }
      ),
    enabled: !!accountId,
  });
}

export function useInvestmentTransactions(
  investmentId: string,
  params?: PaginationParams
) {
  return useQuery({
    queryKey: investmentKeys.transactions(investmentId, params),
    queryFn: () =>
      apiClient.get<PageResponse<InvestmentTransaction>>(
        `/api/v1/investments/${investmentId}/transactions`,
        { ...params }
      ),
    enabled: !!investmentId,
  });
}

export function useAddInvestment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: AddInvestmentRequest) => {
      const res = await apiClient.post<InvestmentResponse>(
        "/api/v1/investments",
        data
      );
      return res.investment;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: investmentKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.all });
    },
  });
}

export function useRecordBuy(investmentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: RecordBuyRequest) => {
      const res = await apiClient.post<InvestmentTransactionResponse>(
        `/api/v1/investments/${investmentId}/buy`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: investmentKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.all });
    },
  });
}

export function useRecordSell(investmentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: RecordSellRequest) => {
      const res = await apiClient.post<InvestmentTransactionResponse>(
        `/api/v1/investments/${investmentId}/sell`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: investmentKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.all });
    },
  });
}

export function useRecordDividend(investmentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: RecordDividendRequest) => {
      const res = await apiClient.post<InvestmentTransactionResponse>(
        `/api/v1/investments/${investmentId}/dividend`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: investmentKeys.all });
    },
  });
}

export function useRecordSplit(investmentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: RecordSplitRequest) => {
      const res = await apiClient.post<InvestmentTransactionResponse>(
        `/api/v1/investments/${investmentId}/split`,
        data
      );
      return res.transaction;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: investmentKeys.all });
    },
  });
}

