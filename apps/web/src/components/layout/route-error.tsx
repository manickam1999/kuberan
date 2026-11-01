"use client";

import { useEffect } from "react";
import Link from "next/link";
import { AlertTriangle, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";

interface RouteErrorProps {
  error: Error & { digest?: string };
  reset: () => void;
  /** Where the secondary "go back" action leads. */
  homeHref?: string;
  homeLabel?: string;
}

/**
 * Shared body for error.tsx boundaries. Shows a generic message (never the
 * error text, which may carry internal details) and a retry that re-renders
 * the segment.
 */
export function RouteError({
  error,
  reset,
  homeHref = "/",
  homeLabel = "Go to dashboard",
}: RouteErrorProps) {
  useEffect(() => {
    // Surface the error in the browser console for debugging.
    console.error(error);
  }, [error]);

  return (
    <div
      role="alert"
      className="flex flex-col items-center justify-center gap-3 py-16 text-center"
    >
      <span className="flex size-11 items-center justify-center rounded-xl bg-destructive/10 text-destructive">
        <AlertTriangle className="size-5" />
      </span>
      <h2 className="text-lg font-semibold">Something went wrong</h2>
      <p className="max-w-sm text-sm text-muted-foreground">
        This page could not be displayed. Try again, and if it keeps happening,
        reload the app.
      </p>
      <div className="flex gap-2 pt-1">
        <Button onClick={reset}>
          <RotateCcw className="size-4" />
          Try again
        </Button>
        <Button asChild variant="outline">
          <Link href={homeHref}>{homeLabel}</Link>
        </Button>
      </div>
    </div>
  );
}
