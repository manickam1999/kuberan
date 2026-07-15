"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useDeleteBudget } from "@/hooks/use-budgets";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { Budget } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return error.message;
  }
  return "An unexpected error occurred";
}

interface DeleteBudgetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  budget: Budget | null;
}

export function DeleteBudgetDialog({
  open,
  onOpenChange,
  budget,
}: DeleteBudgetDialogProps) {
  const [error, setError] = useState("");
  const deleteBudget = useDeleteBudget();
  const isDeleting = deleteBudget.isPending;

  function handleDelete() {
    if (!budget) return;
    setError("");

    deleteBudget.mutate(budget.id, {
      onSuccess: () => {
        toast.success(`Budget "${budget.name}" deleted`);
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
          <DialogTitle>Delete Budget</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &quot;{budget?.name}&quot;? This
            cannot be undone.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
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
