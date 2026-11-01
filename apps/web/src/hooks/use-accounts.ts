import {
  keepPreviousData,
  useQuery,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Account } from "@/types/models";
import type {
  PageResponse,
  PaginationParams,
  AccountResponse,
  CreateCashAccountRequest,
  CreateInvestmentAccountRequest,
  CreateCreditCardAccountRequest,
  CreateFixedDepositAccountRequest,
  CloseFixedDepositRequest,
  UpdateAccountRequest,
} from "@/types/api";

export const accountKeys = {
  all: ["accounts"] as const,
  lists: () => [...accountKeys.all, "list"] as const,
  list: (params?: PaginationParams) =>
    [...accountKeys.lists(), params] as const,
  details: () => [...accountKeys.all, "detail"] as const,
  detail: (id: string) => [...accountKeys.details(), id] as const,
};

export function useAccounts(params?: PaginationParams) {
  return useQuery({
    queryKey: accountKeys.list(params),
    queryFn: () =>
      apiClient.get<PageResponse<Account>>("/api/v1/accounts", {
        ...params,
      }),
    placeholderData: keepPreviousData,
  });
}

async function fetchAccount(id: string): Promise<Account> {
  const res = await apiClient.get<AccountResponse>(`/api/v1/accounts/${id}`);
  return res.account;
}

/** An account already present in a cached accounts list. */
function accountFromLists(
  queryClient: QueryClient,
  id: string
): Account | undefined {
  for (const [, page] of queryClient.getQueriesData<PageResponse<Account>>({
    queryKey: accountKeys.lists(),
  })) {
    const found = page?.data?.find((a) => a.id === id);
    if (found) return found;
  }
  return undefined;
}

export function useAccount(id: string) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: accountKeys.detail(id),
    queryFn: () => fetchAccount(id),
    enabled: !!id,
    // Coming from the accounts list (the usual path), render the header and
    // start the type-specific queries immediately instead of waiting for the
    // detail round trip.
    placeholderData: () => accountFromLists(queryClient, id),
  });
}


export function useCreateCashAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateCashAccountRequest) => {
      const res = await apiClient.post<AccountResponse>(
        "/api/v1/accounts/cash",
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
    },
  });
}

export function useCreateInvestmentAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateInvestmentAccountRequest) => {
      const res = await apiClient.post<AccountResponse>(
        "/api/v1/accounts/investment",
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
    },
  });
}

export function useCreateCreditCardAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateCreditCardAccountRequest) => {
      const res = await apiClient.post<AccountResponse>(
        "/api/v1/accounts/credit-card",
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
    },
  });
}

export function useUpdateAccount(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: UpdateAccountRequest) => {
      const res = await apiClient.put<AccountResponse>(
        `/api/v1/accounts/${id}`,
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
      queryClient.invalidateQueries({ queryKey: accountKeys.detail(id) });
    },
  });
}

export function useCreateFixedDepositAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CreateFixedDepositAccountRequest) => {
      const res = await apiClient.post<AccountResponse>(
        "/api/v1/accounts/fixed-deposit",
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
    },
  });
}

export function useCloseFixedDeposit(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (data: CloseFixedDepositRequest) => {
      const res = await apiClient.post<AccountResponse>(
        `/api/v1/accounts/${id}/fixed-deposit/close`,
        data
      );
      return res.account;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountKeys.lists() });
      queryClient.invalidateQueries({ queryKey: accountKeys.detail(id) });
    },
  });
}
