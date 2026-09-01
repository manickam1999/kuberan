"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useDeleteRule } from "@/hooks/use-rules";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

interface DeleteRuleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule: TransactionRule | null;
}

export function DeleteRuleDialog({
  open,
  onOpenChange,
  rule,
}: DeleteRuleDialogProps) {
  const [error, setError] = useState("");
  const deleteRule = useDeleteRule();
  const isDeleting = deleteRule.isPending;

  function handleDelete() {
    if (!rule) return;
    setError("");

    deleteRule.mutate(rule.id, {
      onSuccess: () => {
        toast.success(`Rule "${rule.name}" deleted`);
        onOpenChange(false);
      },
      onError: (err) => setError(getErrorMessage(err)),
    });
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) setError("");
    onOpenChange(nextOpen);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Delete rule</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &quot;{rule?.name}&quot;? Existing
            transactions keep their categories; only future auto-categorization
            stops.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isDeleting}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
