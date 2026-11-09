import { useEffect, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { PageResponse } from "@/types/api";
import type { Transaction } from "@/types/models";
import { transactionKeys } from "./use-transactions";

/** Fewest characters that start a search. */
export const SEARCH_MIN_CHARS = 2;
/** The API rejects a longer `q`. */
const SEARCH_MAX_CHARS = 100;

/** `value`, but only after it has stopped changing for `delayMs`. */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}

/**
 * Transactions whose description, notes or amount match `query`
 * (`GET /transactions?q=…&page_size=5`). Idle until the query has
 * `SEARCH_MIN_CHARS` characters; the previous results stay while the next
 * ones load so the palette doesn't flicker.
 */
export function useTransactionSearch(query: string, pageSize = 5) {
  const q = query.trim().slice(0, SEARCH_MAX_CHARS);
  const enabled = q.length >= SEARCH_MIN_CHARS;
  return useQuery({
    queryKey: transactionKeys.userList({ q, page_size: pageSize }),
    queryFn: () =>
      apiClient.get<PageResponse<Transaction>>("/api/v1/transactions", {
        q,
        page_size: pageSize,
      }),
    enabled,
    placeholderData: keepPreviousData,
  });
}
