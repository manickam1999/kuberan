import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Reminder } from "@/types/models";
import type { ReminderResponse, RemindersResponse } from "@/types/api";

export const reminderKeys = {
  all: ["reminders"] as const,
  forAccount: (accountId: string) =>
    [...reminderKeys.all, "account", accountId] as const,
};

/**
 * The upcoming reminders of one account (unsent or not yet due), soonest first.
 * The API keeps them in step with the account's due day or maturity date.
 */
export function useAccountReminders(accountId: string) {
  return useQuery({
    queryKey: reminderKeys.forAccount(accountId),
    queryFn: async (): Promise<Reminder[]> => {
      const res = await apiClient.get<RemindersResponse>("/api/v1/reminders", {
        ref_id: accountId,
      });
      return res.reminders;
    },
    enabled: !!accountId,
  });
}

/** Switch an account's reminder on or off. */
export function useSetReminderEnabled(accountId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, enabled }: { id: string; enabled: boolean }) => {
      const res = await apiClient.patch<ReminderResponse>(
        `/api/v1/reminders/${id}`,
        { enabled },
      );
      return res.reminder;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: reminderKeys.forAccount(accountId),
      });
    },
  });
}
