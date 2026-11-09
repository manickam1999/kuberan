"use client";

import { cn } from "@/lib/utils";
import type { BudgetStatus } from "@/lib/budget-pace";

const STATUS_FILL: Record<BudgetStatus, string> = {
  on_track: "var(--primary)",
  close: "var(--warning)",
  over: "var(--negative)",
};

/**
 * Budget progress bar with a pace marker: a thin tick where an even spend
 * would be by now. The fill is capped at 100%; `color` overrides the
 * on-track fill (e.g. the category colour).
 */
export function BudgetPaceBar({
  percentage,
  elapsed,
  status,
  color,
  thick,
  label,
  className,
}: {
  /** spent / budgeted * 100, may exceed 100. */
  percentage: number;
  /** Fraction of the period elapsed, 0 to 1; omit to hide the marker. */
  elapsed?: number;
  status: BudgetStatus;
  color?: string;
  thick?: boolean;
  /** Accessible name; the bar is announced as a progress bar. */
  label: string;
  className?: string;
}) {
  const width = Math.min(Math.max(percentage, 0), 100);
  const fill = status === "on_track" && color ? color : STATUS_FILL[status];
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(width)}
      className={cn(
        "bg-muted relative w-full rounded-full",
        thick ? "h-2.5" : "h-1.5",
        className
      )}
    >
      <div
        className="h-full rounded-full transition-[width]"
        style={{ width: `${width}%`, backgroundColor: fill }}
      />
      {elapsed !== undefined && (
        <span
          aria-hidden
          title="Where an even spend would be by today"
          className="bg-foreground/70 absolute top-[-3px] bottom-[-3px] w-0.5 rounded-full"
          style={{
            left: `calc(${Math.min(Math.max(elapsed, 0), 1) * 100}% - 1px)`,
          }}
        />
      )}
    </div>
  );
}
