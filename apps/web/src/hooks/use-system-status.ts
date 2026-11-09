import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { SystemStatus } from "@/types/api";

export const systemStatusKeys = {
  all: ["system-status"] as const,
  detail: () => [...systemStatusKeys.all, "detail"] as const,
};

/** Freshness of prices, FX rates, snapshots and backups (sidebar host chip). */
export function useSystemStatus() {
  return useQuery({
    queryKey: systemStatusKeys.detail(),
    queryFn: () => apiClient.get<SystemStatus>("/api/v1/system/status"),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
  });
}
