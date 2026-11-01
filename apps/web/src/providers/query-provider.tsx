"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import type { ReactNode } from "react";
import { ApiClientError } from "@/lib/api-client";

/**
 * Retry once, but only for failures a retry can fix (network errors, 5xx).
 * A 4xx (validation, not found, forbidden, signed out) fails the same way
 * every time, so retrying only delays the error state.
 */
function shouldRetry(failureCount: number, error: unknown): boolean {
  if (
    error instanceof ApiClientError &&
    error.status >= 400 &&
    error.status < 500
  ) {
    return false;
  }
  return failureCount < 1;
}

export function QueryProvider({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5 * 60 * 1000, // 5 minutes
            retry: shouldRetry,
            // Kept on: with a 5-minute staleTime, focus refetches only stale
            // queries, which keeps balances current after the user records a
            // transaction elsewhere (another tab, the Telegram bot, MCP)
            // without hammering the API on every tab switch.
            refetchOnWindowFocus: true,
          },
          mutations: {
            // Mutations are not idempotent; never replay them automatically.
            retry: false,
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}
