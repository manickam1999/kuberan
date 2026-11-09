"use client";

import { Download, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { IbkrImport } from "@/components/settings/ibkr-import";
import { SettingsSection } from "@/components/settings/settings-section";
import { useDataExport } from "@/hooks/use-data-export";
import { useSystemStatus } from "@/hooks/use-system-status";
import { ApiClientError } from "@/lib/api-client";
import { formatDateTime } from "@/lib/format";
import { formatRelativeTime } from "@/lib/relative-time";

function rateLimitMessage(err: ApiClientError): string {
  const secs = err.details?.retry_after;
  return typeof secs === "number"
    ? `You can export once a minute. Try again in ${secs} second${secs === 1 ? "" : "s"}.`
    : "You can export once a minute. Try again shortly.";
}

function ExportCard() {
  const exportData = useDataExport();

  function run() {
    exportData.mutate(undefined, {
      onSuccess: () => toast.success("Export downloaded"),
      onError: (err) => {
        if (err instanceof ApiClientError && err.status === 429) {
          toast.error("Slow down", { description: rateLimitMessage(err) });
        } else {
          toast.error("Couldn't export your data", {
            description: err instanceof ApiClientError ? err.message : undefined,
          });
        }
      },
    });
  }

  return (
    <SettingsSection
      title="Export"
      description="Download everything as a zip of CSV files, one per table. It's your data."
    >
      <Button onClick={run} disabled={exportData.isPending}>
        <Download className="size-4" />
        {exportData.isPending ? "Preparing..." : "Export all data"}
      </Button>
    </SettingsSection>
  );
}

function StatusCard() {
  const { data, isLoading, isError, refetch, isRefetching } = useSystemStatus();

  const rows: { label: string; at: string | null | undefined; never: string }[] =
    [
      { label: "Last backup", at: data?.last_backup_at, never: "No backup yet" },
      { label: "Security prices", at: data?.latest_price_at, never: "None yet" },
      { label: "Exchange rates", at: data?.latest_fx_rate_at, never: "None yet" },
      {
        label: "Portfolio snapshot",
        at: data?.latest_snapshot_at,
        never: "None yet",
      },
    ];

  return (
    <SettingsSection
      title="Backups and data freshness"
      description="When the server last backed up and last refreshed prices, exchange rates and your net-worth history."
    >
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-5" />
          <Skeleton className="h-5" />
          <Skeleton className="h-5" />
        </div>
      ) : isError ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-destructive">
            Couldn&apos;t load the system status.
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void refetch()}
            disabled={isRefetching}
          >
            <RefreshCw className="size-4" />
            Try again
          </Button>
        </div>
      ) : (
        <dl className="divide-y divide-border/60">
          {rows.map((r) => (
            <div
              key={r.label}
              className="flex items-center justify-between gap-4 py-2.5 first:pt-0 last:pb-0"
            >
              <dt className="text-sm text-muted-foreground">{r.label}</dt>
              <dd className="text-right text-sm font-medium">
                {r.at ? (
                  <time dateTime={r.at} title={formatDateTime(r.at)}>
                    {formatRelativeTime(r.at)}
                  </time>
                ) : (
                  <span className="text-muted-foreground">{r.never}</span>
                )}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </SettingsSection>
  );
}

/** Settings > Data and export (prototype `prefs-data`). */
export function DataSettings() {
  return (
    <div className="space-y-4">
      <ExportCard />
      <StatusCard />
      <IbkrImport />
    </div>
  );
}
