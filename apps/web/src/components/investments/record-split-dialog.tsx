"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useRecordSplit } from "@/hooks/use-investments";
import { toRFC3339 } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return error.message;
  }
  return "An unexpected error occurred";
}

interface RecordSplitDialogProps {
  investmentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function RecordSplitDialog({
  investmentId,
  open,
  onOpenChange,
}: RecordSplitDialogProps) {
  const [date, setDate] = useState("");
  const [splitRatio, setSplitRatio] = useState("");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");

  const recordSplit = useRecordSplit(investmentId);
  const isSubmitting = recordSplit.isPending;

  function resetForm() {
    setDate("");
    setSplitRatio("");
    setNotes("");
    setError("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (!date) {
      setError("Date is required");
      return;
    }

    const ratio = parseFloat(splitRatio);
    if (!ratio || ratio <= 0) {
      setError("Split ratio must be greater than zero");
      return;
    }

    recordSplit.mutate(
      {
        date: toRFC3339(date),
        split_ratio: ratio,
        notes: notes.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.success("Split recorded");
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Record Split</DialogTitle>
          <DialogDescription>
            Record a split. A 2:1 split means each unit becomes 2 units.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="split-date">Date</Label>
            <Input
              id="split-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="split-ratio">Split Ratio</Label>
            <Input
              id="split-ratio"
              type="number"
              step="any"
              min="0"
              placeholder="e.g., 2 for a 2:1 split"
              value={splitRatio}
              onChange={(e) => setSplitRatio(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="split-notes">Notes (optional)</Label>
            <Textarea
              id="split-notes"
              placeholder="Optional notes..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Recording..." : "Record Split"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
