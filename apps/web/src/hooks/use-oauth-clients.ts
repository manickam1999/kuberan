import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type {
  AllAppActivityItem,
  AppActivityItem,
  ConnectedAppsResponse,
  PageResponse,
  PaginationParams,
} from "@/types/api";

export const oauthClientKeys = {
  all: ["oauth-clients"] as const,
  list: () => [...oauthClientKeys.all, "list"] as const,
  activity: () => [...oauthClientKeys.all, "activity"] as const,
  appActivity: (clientId: string, params: PaginationParams = {}) =>
    [...oauthClientKeys.activity(), "app", clientId, params] as const,
  allActivity: (params: PaginationParams = {}) =>
    [...oauthClientKeys.activity(), "all", params] as const,
};

export function useConnectedApps() {
  return useQuery({
    queryKey: oauthClientKeys.list(),
    queryFn: async () => {
      const res = await apiClient.get<ConnectedAppsResponse>(
        "/api/v1/oauth/clients"
      );
      return res.clients;
    },
  });
}

/** Changes one connected app made, newest first. Pass null to disable. */
export function useAppActivity(
  clientId: string | null,
  params: PaginationParams = {}
) {
  return useQuery({
    queryKey: oauthClientKeys.appActivity(clientId ?? "", params),
    queryFn: () =>
      apiClient.get<PageResponse<AppActivityItem>>(
        `/api/v1/oauth/clients/${encodeURIComponent(clientId ?? "")}/activity`,
        { ...params }
      ),
    enabled: clientId !== null && clientId !== "",
  });
}

/** Changes made by all of the user's connected apps, newest first. */
export function useAllAppActivity(params: PaginationParams = {}) {
  return useQuery({
    queryKey: oauthClientKeys.allActivity(params),
    queryFn: () =>
      apiClient.get<PageResponse<AllAppActivityItem>>(
        "/api/v1/oauth/clients/activity",
        { ...params }
      ),
  });
}

export function useDisconnectApp() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (clientId: string) =>
      apiClient.del<void>(
        `/api/v1/oauth/clients/${encodeURIComponent(clientId)}`
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: oauthClientKeys.all });
    },
  });
}
