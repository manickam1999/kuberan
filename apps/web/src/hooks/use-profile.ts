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
  return useQuery({
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
