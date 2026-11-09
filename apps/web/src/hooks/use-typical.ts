import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { transactionKeys } from "@/hooks/use-transactions";
import type { TypicalMonth } from "@/types/api";

/** Under the transactions prefix so any transaction change refetches it. */
export const typicalKeys = {
  typical: (months: number) =>
    [...transactionKeys.all, "typical", months] as const,
};

/** What a normal month looks like (average of the last complete months). */
export function useTypical(months = 6) {
  return useQuery({
    queryKey: typicalKeys.typical(months),
    queryFn: () =>
      apiClient.get<{ data: TypicalMonth }>("/api/v1/analytics/typical", {
        months,
      }),
    select: (res) => res.data,
    staleTime: 5 * 60_000,
  });
}
