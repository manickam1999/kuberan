import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { PageResponse } from "@/types/api";

/** Cached data captured before an optimistic update, for rollback. */
export type CacheSnapshot = Array<[QueryKey, unknown]>;

/** Mutation context carrying the rollback snapshot. */
export interface OptimisticContext {
  snapshot: CacheSnapshot;
}

/**
 * Cancel in-flight fetches under the given key prefixes (so they cannot
 * overwrite the optimistic data) and capture their current data.
 */
export async function snapshotQueries(
  queryClient: QueryClient,
  prefixes: QueryKey[]
): Promise<CacheSnapshot> {
  await Promise.all(
    prefixes.map((queryKey) => queryClient.cancelQueries({ queryKey }))
  );
  return prefixes.flatMap((queryKey) =>
    queryClient.getQueriesData<unknown>({ queryKey })
  );
}

/** Put back the data captured by {@link snapshotQueries}. */
export function restoreSnapshot(
  queryClient: QueryClient,
  context: OptimisticContext | undefined
): void {
  if (!context) return;
  for (const [queryKey, data] of context.snapshot) {
    queryClient.setQueryData(queryKey, data);
  }
}

/**
 * Apply `update` to every cached page under `prefix`. `update` receives the
 * page and the query key (so list filters can be read from it) and returns the
 * new page, or the same object to leave it unchanged.
 */
export function updatePages<T>(
  queryClient: QueryClient,
  prefix: QueryKey,
  update: (page: PageResponse<T>, queryKey: QueryKey) => PageResponse<T>
): void {
  for (const [queryKey, page] of queryClient.getQueriesData<PageResponse<T>>({
    queryKey: prefix,
  })) {
    if (!page || !Array.isArray(page.data)) continue;
    const next = update(page, queryKey);
    if (next !== page) queryClient.setQueryData(queryKey, next);
  }
}

/** Remove the item with `id` from a page, adjusting the totals. */
export function removeFromPage<T extends { id: string }>(
  page: PageResponse<T>,
  id: string
): PageResponse<T> {
  const data = page.data.filter((item) => item.id !== id);
  if (data.length === page.data.length) return page;
  return { ...page, data, total_items: Math.max(0, page.total_items - 1) };
}

/** Apply `patch` to the item with `id` in a page. */
export function patchInPage<T extends { id: string }>(
  page: PageResponse<T>,
  id: string,
  patch: (item: T) => T
): PageResponse<T> {
  let changed = false;
  const data = page.data.map((item) => {
    if (item.id !== id) return item;
    changed = true;
    return patch(item);
  });
  return changed ? { ...page, data } : page;
}

/** Client-side id for an item that only exists optimistically. */
export function optimisticId(): string {
  const random =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random()}`;
  return `optimistic-${random}`;
}
