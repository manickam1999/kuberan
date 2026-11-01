import { Skeleton } from "@/components/ui/skeleton";

/**
 * Route-level loading placeholder (used by loading.tsx files). `list` suits
 * index pages (header, stat row, table); `detail` suits [id] pages (back link,
 * title block, chart, table).
 */
export function PageSkeleton({ variant = "list" }: { variant?: "list" | "detail" }) {
  if (variant === "detail") {
    return (
      <div className="space-y-6" aria-busy="true" aria-label="Loading">
        <Skeleton className="h-8 w-36" />
        <div className="space-y-2">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
        <Skeleton className="h-[280px] w-full" />
        <Skeleton className="h-[320px] w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading">
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-2">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
        <Skeleton className="h-9 w-32" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-24 w-full" />
        ))}
      </div>
      <div className="space-y-2">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    </div>
  );
}
