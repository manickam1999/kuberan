import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";

/** Response of `GET /api/v1/fx-rates/latest`. */
export interface FxLatest {
  base: string;
  quote: string;
  /** `1 base = rate quote`, a decimal string. */
  rate: string;
  recorded_at: string;
  stale: boolean;
  /** `identity`, `direct`, `inverse` or `pivot:USD`. */
  path: string;
}

export const fxRateKeys = {
  all: ["fx-rates"] as const,
  latest: (base: string, quote: string) =>
    [...fxRateKeys.all, "latest", base, quote] as const,
};

/**
 * The current `base -> quote` rate (direct, inverse or USD-pivot). Disabled
 * until both currencies are known or when they are equal. A 503
 * `FX_RATE_UNAVAILABLE` surfaces as `error` and the query is not retried.
 */
export function useFxLatest(base?: string, quote?: string) {
  return useQuery({
    queryKey: fxRateKeys.latest(base ?? "", quote ?? ""),
    queryFn: () =>
      apiClient.get<FxLatest>("/api/v1/fx-rates/latest", {
        base: base ?? "",
        quote: quote ?? "",
      }),
    enabled: !!base && !!quote && base !== quote,
    staleTime: 60_000,
    retry: false,
  });
}

/**
 * `amountCents` of `from` converted to `to` at `rate` (a decimal string),
 * rounded to whole cents. Returns null for an unusable rate. Both currencies
 * are assumed to have 2 decimals (the API only supports those).
 */
export function convertCents(
  amountCents: number,
  rate: string | undefined
): number | null {
  const r = rate ? Number(rate) : NaN;
  if (!Number.isFinite(r) || r <= 0) return null;
  return Math.round(amountCents * r);
}
