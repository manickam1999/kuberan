import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type {
  ChangePasswordRequest,
  RevokedSessionsResponse,
  SessionsResponse,
} from "@/types/api";

export const sessionKeys = {
  all: ["auth-sessions"] as const,
  list: () => [...sessionKeys.all, "list"] as const,
};

/** The user's signed-in devices (live refresh sessions); one has `current: true`. */
export function useSessions() {
  return useQuery({
    queryKey: sessionKeys.list(),
    queryFn: async () => {
      const res = await apiClient.get<SessionsResponse>("/api/v1/auth/sessions");
      return res.sessions;
    },
  });
}

/**
 * Ends another device's session. The current session is refused (400
 * `CURRENT_SESSION`; use logout), an unknown one is 404 `SESSION_NOT_FOUND`.
 */
export function useRevokeSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiClient.del<void>(
        `/api/v1/auth/sessions/${encodeURIComponent(sessionId)}`
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}

/** Ends every session except the current one. */
export function useRevokeOtherSessions() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () =>
      apiClient.post<RevokedSessionsResponse>(
        "/api/v1/auth/sessions/revoke-others"
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}

/**
 * Changes the password. Other devices are signed out (this one stays). A wrong
 * current password surfaces as `ApiClientError` code `INVALID_CURRENT_PASSWORD`
 * (400), a locked account as `ACCOUNT_LOCKED` (423).
 */
export function useChangePassword() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: ChangePasswordRequest) =>
      apiClient.post<RevokedSessionsResponse>("/api/v1/auth/password", body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}
