import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { DemoResult } from "@/types/api";

/**
 * Loads the sample household (POST /demo/load). Only offered when the server
 * reports `demo_enabled` in the system status. It creates accounts, so every
 * cached query is invalidated afterwards.
 */
export function useLoadDemo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiClient.post<DemoResult>("/api/v1/demo/load"),
    onSuccess: () => {
      void queryClient.invalidateQueries();
    },
  });
}

/** Removes the sample household for good (DELETE /demo). */
export function useRemoveDemo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiClient.del<DemoResult>("/api/v1/demo"),
    onSuccess: () => {
      void queryClient.invalidateQueries();
    },
  });
}
