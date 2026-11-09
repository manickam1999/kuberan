"use client";

import { Download } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useLoadDemo } from "@/hooks/use-demo";
import { useSystemStatus } from "@/hooks/use-system-status";
import { ApiClientError } from "@/lib/api-client";

/** Why a load was refused, in the user's words. */
export function demoLoadErrorMessage(err: unknown): string {
  if (err instanceof ApiClientError) {
    if (err.code === "DEMO_HAS_DATA") {
      return "Sample data can only be loaded before you add your own transactions.";
    }
    if (err.code === "DEMO_ALREADY_LOADED") {
      return "The sample household is already loaded.";
    }
    if (err.status === 404) return "Sample data isn't enabled on this server.";
    return err.message;
  }
  return "Something went wrong. Please try again.";
}

/**
 * "Explore with sample data" on the onboarding checklist (prototype `welcome`).
 * Rendered only when the server reports `demo_enabled` (DEMO_ENABLED) and the
 * sample household isn't loaded yet.
 */
export function SampleDataCard() {
  const { data: status } = useSystemStatus();
  const load = useLoadDemo();
  if (!status?.demo_enabled || status.demo_loaded) return null;

  function run() {
    load.mutate(undefined, {
      onSuccess: () =>
        toast.success("Sample household loaded", {
          description: "Remove it any time in Settings, Data.",
        }),
      onError: (err) =>
        toast.error("Couldn't load the sample data", {
          description: demoLoadErrorMessage(err),
        }),
    });
  }

  return (
    <Card className="gap-3 px-5 py-4">
      <h2 className="text-sm font-semibold">Explore with sample data</h2>
      <p className="text-sm text-muted-foreground">
        Want to look around first? Load a sample household with about three
        months of activity. You can remove it with one click in Settings.
      </p>
      <div>
        <Button variant="outline" onClick={run} disabled={load.isPending}>
          <Download className="size-4" />
          {load.isPending ? "Loading..." : "Load demo data"}
        </Button>
      </div>
    </Card>
  );
}
