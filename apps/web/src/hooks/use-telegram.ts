import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { UpdateReminderSettingsRequest } from "@/types/api";

interface TelegramLink {
  id: string;
  user_id: string;
  telegram_user_id: number;
  telegram_username?: string;
  telegram_first_name?: string;
  is_active: boolean;
  last_message_at?: string;
  message_count: number;
  /** Per-kind reminder switches (default on). */
  remind_card_due: boolean;
  remind_loan_due: boolean;
  remind_fd_maturity: boolean;
}

interface GenerateCodeResponse {
  link_code: string;
  expires_at: string;
}

export function useTelegramLink() {
  return useQuery<{ link: TelegramLink }>({
    queryKey: ["telegram", "link"],
    queryFn: () => apiClient.get("/api/v1/telegram/link"),
    retry: false,
  });
}

export function useGenerateLinkCode() {
  const queryClient = useQueryClient();

  return useMutation<GenerateCodeResponse>({
    mutationFn: () => apiClient.post("/api/v1/telegram/generate-code"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["telegram"] });
    },
  });
}

/** Switch card due, loan due and deposit maturity reminders on or off. */
export function useUpdateReminderSettings() {
  const queryClient = useQueryClient();

  return useMutation<{ link: TelegramLink }, Error, UpdateReminderSettingsRequest>({
    mutationFn: (data) =>
      apiClient.put("/api/v1/telegram/reminder-settings", data),
    onSuccess: (res) => {
      queryClient.setQueryData(["telegram", "link"], res);
    },
  });
}

export function useUnlinkTelegram() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => apiClient.del("/api/v1/telegram/unlink"),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ["telegram"] });
    },
  });
}
