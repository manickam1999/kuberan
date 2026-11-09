"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { demoLoadErrorMessage } from "@/components/dashboard/sample-data-card";
import { SettingsRow, SettingsSection } from "@/components/settings/settings-section";
import { useLoadDemo, useRemoveDemo } from "@/hooks/use-demo";
import { useSystemStatus } from "@/hooks/use-system-status";
import { ApiClientError } from "@/lib/api-client";

function removeErrorMessage(err: unknown): string {
  if (err instanceof ApiClientError) {
    if (err.code === "DEMO_IN_USE") {
      return "Something outside the sample household still depends on it, such as a receipt or a transfer to one of your own accounts. Remove that first.";
    }
    return err.message;
  }
  return "Something went wrong. Please try again.";
}

/**
 * Settings > Data: remove (or, before any real data exists, load) the sample
 * household. Rendered only when the server reports `demo_enabled`.
 */
export function DemoDataCard() {
  const { data: status } = useSystemStatus();
  const load = useLoadDemo();
  const remove = useRemoveDemo();
  const [confirmOpen, setConfirmOpen] = useState(false);

  if (!status?.demo_enabled) return null;

  function runRemove() {
    remove.mutate(undefined, {
      onSuccess: () => {
        setConfirmOpen(false);
        toast.success("Sample data removed");
      },
      onError: (err) => {
        setConfirmOpen(false);
        toast.error("Couldn't remove the sample data", {
          description: removeErrorMessage(err),
        });
      },
    });
  }

  function runLoad() {
    load.mutate(undefined, {
      onSuccess: () => toast.success("Sample household loaded"),
      onError: (err) =>
        toast.error("Couldn't load the sample data", {
          description: demoLoadErrorMessage(err),
        }),
    });
  }

  return (
    <>
      <SettingsSection
        title={status.demo_loaded ? "Delete demo data" : "Sample data"}
        description={
          status.demo_loaded
            ? "Removes the sample household for good: its accounts, transactions, budget, holdings and receivables, and anything you added inside those accounts. Your own accounts stay."
            : "Load a sample household with about three months of activity to look around. Only possible before you add your own transactions."
        }
      >
        <SettingsRow
          title={status.demo_loaded ? "The sample household is loaded" : "No sample data"}
        >
          {status.demo_loaded ? (
            <Button
              variant="outline"
              className="text-destructive hover:text-destructive"
              onClick={() => setConfirmOpen(true)}
              disabled={remove.isPending}
            >
              Remove demo data
            </Button>
          ) : (
            <Button variant="outline" onClick={runLoad} disabled={load.isPending}>
              {load.isPending ? "Loading..." : "Load demo data"}
            </Button>
          )}
        </SettingsRow>
      </SettingsSection>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove the sample data?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes the sample accounts, categories,
              transactions, budget, holdings and receivables, and anything you
              added inside those accounts. It can&apos;t be undone. Your own
              accounts and entries are not touched.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={remove.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={remove.isPending}
              onClick={(e) => {
                e.preventDefault();
                runRemove();
              }}
            >
              {remove.isPending ? "Removing..." : "Remove"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
