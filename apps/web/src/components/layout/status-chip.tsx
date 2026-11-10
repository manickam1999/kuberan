"use client";

import { useSyncExternalStore } from "react";
import { useSystemStatus } from "@/hooks/use-system-status";
import { cn } from "@/lib/utils";
import type { SystemStatus } from "@/types/api";

/** Prices older than this show the warning dot. */
const STALE_PRICES_MS = 48 * 60 * 60 * 1000;

const noopSubscribe = () => () => {};

/** True after hydration, so local-time text never mismatches the server. */
function useIsClient(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false
  );
}

/** "06:00" for today, otherwise "8 Nov". */
function shortStamp(iso: string, now: Date): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "unknown";
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString("en-GB", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

function summary(status: SystemStatus, wide: boolean, now: Date): string {
  const prices = status.latest_price_at
    ? `${wide ? "Prices updated" : "prices"} ${shortStamp(status.latest_price_at, now)}`
    : "no prices yet";
  const backup = status.last_backup_at
    ? `backup ${shortStamp(status.last_backup_at, now)}`
    : null;
  return [prices, backup].filter(Boolean).join(" · ");
}

function isStale(status: SystemStatus | undefined, now: Date): boolean {
  if (!status?.latest_price_at) return false;
  return (
    now.getTime() - new Date(status.latest_price_at).getTime() >
    STALE_PRICES_MS
  );
}

/**
 * Host status chip (prototype `.host`): "Self-hosted" plus when prices and the
 * backup last ran, fed by `GET /system/status`. The dot turns amber when
 * prices are stale or the status can't be read.
 */
export function StatusChip({
  wide = false,
  className,
}: {
  wide?: boolean;
  className?: string;
}) {
  const { data, isError } = useSystemStatus();
  const client = useIsClient();
  const now = new Date();
  const warn = isError || (client && isStale(data, now));

  return (
    <div
      data-slot="status-chip"
      className={cn(
        "text-muted-foreground grid gap-0.5 border px-3 py-2.5 text-[11.5px]",
        wide ? "rounded-card" : "rounded-[10px]",
        className
      )}
    >
      <b className="text-foreground flex items-center gap-1.5 text-xs font-medium">
        <i
          aria-hidden
          className={cn(
            "size-1.5 rounded-full",
            warn ? "bg-warning" : "bg-positive"
          )}
        />
        Self-hosted
      </b>
      <span>
        {isError
          ? "status unavailable"
          : data && client
            ? summary(data, wide, now)
            : "checking…"}
      </span>
    </div>
  );
}
