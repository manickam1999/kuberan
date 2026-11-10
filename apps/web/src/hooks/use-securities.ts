import {
  keepPreviousData,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { useCallback } from "react";
import { apiClient } from "@/lib/api-client";
import type { Security, SecurityPrice } from "@/types/models";
import type {
  PageResponse,
  PaginationParams,
  SecurityResponse,
  SecurityFilters,
} from "@/types/api";

export const securityKeys = {
  all: ["securities"] as const,
  lists: () => [...securityKeys.all, "list"] as const,
  list: (params?: SecurityFilters) =>
    [...securityKeys.lists(), params] as const,
  details: () => [...securityKeys.all, "detail"] as const,
  detail: (id: string) => [...securityKeys.details(), id] as const,
  prices: (id: string, from: string, to: string) =>
    [...securityKeys.all, "prices", id, from, to] as const,
  series: (id: string, from: string, to: string) =>
    [...securityKeys.all, "series", id, from, to] as const,
  everything: (search: string) =>
    [...securityKeys.all, "everything", search] as const,
};

export function useSecurities(params?: SecurityFilters) {
  return useQuery({
    queryKey: securityKeys.list(params),
    queryFn: () =>
      apiClient.get<PageResponse<Security>>("/api/v1/securities", {
        ...params,
      }),
    // Keep results on screen while a new page or search loads.
    placeholderData: keepPreviousData,
  });
}

async function fetchSecurity(id: string): Promise<Security> {
  const res = await apiClient.get<SecurityResponse>(`/api/v1/securities/${id}`);
  return res.security;
}

/** A security already present in a cached list, to show while its detail loads. */
function securityFromLists(
  queryClient: QueryClient,
  id: string
): Security | undefined {
  for (const [, page] of queryClient.getQueriesData<PageResponse<Security>>({
    queryKey: securityKeys.lists(),
  })) {
    const found = page?.data?.find((s) => s.id === id);
    if (found) return found;
  }
  return undefined;
}

export function useSecurity(id: string) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: securityKeys.detail(id),
    queryFn: () => fetchSecurity(id),
    enabled: !!id,
    placeholderData: () => securityFromLists(queryClient, id),
  });
}

/** Returns a callback that warms the detail cache, e.g. on row hover. */
export function usePrefetchSecurity() {
  const queryClient = useQueryClient();
  return useCallback(
    (id: string) => {
      void queryClient.prefetchQuery({
        queryKey: securityKeys.detail(id),
        queryFn: () => fetchSecurity(id),
      });
    },
    [queryClient]
  );
}

export function useSecurityPriceHistory(
  id: string,
  from: string,
  to: string,
  params?: PaginationParams
) {
  return useQuery({
    queryKey: securityKeys.prices(id, from, to),
    queryFn: () =>
      apiClient.get<PageResponse<SecurityPrice>>(
        `/api/v1/securities/${id}/prices`,
        { from_date: from, to_date: to, ...params }
      ),
    enabled: !!id && from.length > 0 && to.length > 0,
  });
}

const PRICE_PAGE_SIZE = 100; // the API maximum
const MAX_PRICE_PAGES = 10;

/**
 * Every price of a security between `from` and `to`, newest first: pages
 * through the price history endpoint (100 a page, at most 1,000 prices).
 */
export function useSecurityPriceSeries(id: string, from: string, to: string) {
  return useQuery({
    queryKey: securityKeys.series(id, from, to),
    queryFn: async () => {
      const all: SecurityPrice[] = [];
      for (let page = 1; page <= MAX_PRICE_PAGES; page++) {
        const res = await apiClient.get<PageResponse<SecurityPrice>>(
          `/api/v1/securities/${id}/prices`,
          { from_date: from, to_date: to, page, page_size: PRICE_PAGE_SIZE }
        );
        all.push(...(res.data ?? []));
        if (page >= res.total_pages) break;
      }
      return all;
    },
    enabled: !!id && from.length > 0 && to.length > 0,
  });
}

const SECURITY_PAGE_SIZE = 100; // the API maximum
const MAX_SECURITY_PAGES = 10;

/**
 * Every security matching `search` (empty: all), symbol order. The list is
 * reference data, so it is fetched whole (at most 1,000) and filtered by type
 * on the client.
 */
export function useAllSecurities(search: string) {
  return useQuery({
    queryKey: securityKeys.everything(search),
    queryFn: async () => {
      const all: Security[] = [];
      for (let page = 1; page <= MAX_SECURITY_PAGES; page++) {
        const res = await apiClient.get<PageResponse<Security>>(
          "/api/v1/securities",
          {
            search: search || undefined,
            page,
            page_size: SECURITY_PAGE_SIZE,
          }
        );
        all.push(...(res.data ?? []));
        if (page >= res.total_pages) break;
      }
      return all;
    },
    placeholderData: keepPreviousData,
  });
}
