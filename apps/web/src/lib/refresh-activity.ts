import type { Query } from "@tanstack/react-query";

/** Query `meta` flag for polls the refresh bar should ignore. */
export interface RefreshMeta {
  /** A routine background poll (e.g. the status chip); never shows the bar. */
  silentRefetch?: boolean;
}

/**
 * True for a query that already shows data and is fetching it again: a
 * focus/reconnect refetch, an invalidation after a mutation, a stale query on
 * mount. First loads are left out because the page already shows skeletons,
 * and so are polls marked `meta: { silentRefetch: true }`.
 */
export function isBackgroundRefetch(
  query: Pick<Query, "state" | "meta">
): boolean {
  if (query.state.fetchStatus !== "fetching") return false;
  if (query.state.data === undefined) return false;
  return (query.meta as RefreshMeta | undefined)?.silentRefetch !== true;
}
