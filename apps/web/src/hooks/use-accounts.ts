import {
  keepPreviousData,
  useQuery,
  useMutation,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { useBaseCurrency } from "@/hooks/use-profile";
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

/** List params; `include_inactive` also returns deactivated accounts. */
export type AccountListParams = PaginationParams & { include_inactive?: boolean };

export function useAccounts(params?: AccountListParams) {
  return useQuery({
    queryKey: accountKeys.list(params),
    queryFn: () =>
      apiClient.get<PageResponse<Account>>("/api/v1/accounts", {
        ...params,
      }),
    placeholderData: keepPreviousData,
  });
}

/**
 * Resolver for an account's currency by id, from the accounts query (shared
 * cache with the accounts page). Falls back to a preloaded relation, then to
 * the user's base currency while nothing is loaded.
 */
export function useAccountCurrencies(): (
  accountId: string | null | undefined,
  preloaded?: Pick<Account, "currency"> | null
) => string {
  // Inactive too: their past transactions still need a currency.
  const { data } = useAccounts({ page_size: 100, include_inactive: true });
  const base = useBaseCurrency();
  // `||`, not `??`: an embedded account the API didn't preload arrives as a
  // zero value with currency "", which must fall through too.
  return (accountId, preloaded) =>
    data?.data.find((a) => a.id === accountId)?.currency ||
    preloaded?.currency ||
    base;
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
