"use client";

import { RouteError } from "@/components/layout/route-error";

// Catches errors outside the dashboard group (auth pages, root layout
// children). Dashboard pages have their own boundary that keeps the sidebar.
export default function RootError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <RouteError error={error} reset={reset} />
    </div>
  );
}
