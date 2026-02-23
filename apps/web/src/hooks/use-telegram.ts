import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";

interface TelegramLink {
  id: number;
  user_id: number;
  telegram_user_id: number;
  telegram_username?: string;
  telegram_first_name?: string;
  is_active: boolean;
  last_message_at?: string;
  message_count: number;
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

export function useUnlinkTelegram() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => apiClient.del("/api/v1/telegram/unlink"),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ["telegram"] });
    },
  });
}
