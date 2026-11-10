import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { useAuth } from "@/hooks/use-auth";
import type { User } from "@/types/models";
import type {
  ProfileResponse,
  UpdateProfileSettingsRequest,
} from "@/types/api";

export const profileKeys = {
  all: ["profile"] as const,
};

export function useProfile() {
  // The session bootstrap already returned the user; show it while the
  // authoritative profile loads.
  const { user } = useAuth();
  return useQuery({
    placeholderData: user ?? undefined,
    queryKey: profileKeys.all,
    queryFn: async () => {
      const res = await apiClient.get<ProfileResponse>("/api/v1/profile");
      return res.user;
    },
  });
}

/** Saves (or, with null, clears) the account preselected for new transactions. */
export function useSetDefaultAccount() {
  const queryClient = useQueryClient();
  const { patchUser } = useAuth();

  return useMutation({
    mutationFn: async (accountId: string | null) => {
      const body: UpdateProfileSettingsRequest = {
        default_account_id: accountId,
      };
      const res = await apiClient.patch<ProfileResponse>(
        "/api/v1/profile",
        body
      );
      return res.user;
    },
    onSuccess: (user) => {
      // Merge only the field this mutation owns, so a concurrent optimistic
      // hide-balances toggle is not rolled back by this response.
      const fields = { default_account_id: user.default_account_id };
      patchUser(fields);
      queryClient.setQueryData<User>(profileKeys.all, (prev) =>
        prev ? { ...prev, ...fields } : user
      );
    },
  });
}

/**
 * Saves the user's name and/or email (`PATCH /profile`). Changing the email
 * needs `current_password`; errors surface as `ApiClientError` codes
 * `INVALID_CURRENT_PASSWORD` (400), `DUPLICATE_EMAIL` (409) or `ACCOUNT_LOCKED`
 * (423).
 */
export function useUpdateProfile() {
  const queryClient = useQueryClient();
  const { patchUser } = useAuth();

  return useMutation({
    mutationFn: async (
      body: Pick<
        UpdateProfileSettingsRequest,
        "first_name" | "last_name" | "email" | "current_password"
      >
    ) => {
      const res = await apiClient.patch<ProfileResponse>(
        "/api/v1/profile",
        body
      );
      return res.user;
    },
    onSuccess: (user) => {
      const fields = {
        first_name: user.first_name,
        last_name: user.last_name,
        email: user.email,
      };
      patchUser(fields);
      queryClient.setQueryData<User>(profileKeys.all, (prev) =>
        prev ? { ...prev, ...fields } : user
      );
    },
  });
}

/** Fallback reporting currency while the profile loads. */
export const DEFAULT_BASE_CURRENCY = "MYR";

/**
 * The user's reporting (base) currency from the profile query; `"MYR"` while
 * the profile is loading or unavailable.
 */
export function useBaseCurrency(): string {
  const { data } = useProfile();
  return data?.base_currency || DEFAULT_BASE_CURRENCY;
}

/**
 * Saves the reporting currency (`PATCH /profile {base_currency}`). Every
 * aggregate changes with it, so on success ALL queries are invalidated. A
 * rejected currency surfaces as an `ApiClientError` (code
 * `UNSUPPORTED_CURRENCY`) in `mutation.error`.
 */
export function useSetBaseCurrency() {
  const queryClient = useQueryClient();
  const { patchUser } = useAuth();

  return useMutation({
    mutationFn: async (baseCurrency: string) => {
      const body: UpdateProfileSettingsRequest = {
        base_currency: baseCurrency,
      };
      const res = await apiClient.patch<ProfileResponse>(
        "/api/v1/profile",
        body
      );
      return res.user;
    },
    onSuccess: (user) => {
      const fields = { base_currency: user.base_currency };
      patchUser(fields);
      queryClient.setQueryData<User>(profileKeys.all, (prev) =>
        prev ? { ...prev, ...fields } : user
      );
      void queryClient.invalidateQueries();
    },
  });
}
