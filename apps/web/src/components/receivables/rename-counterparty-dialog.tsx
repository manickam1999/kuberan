"use client";

import { useState } from "react";
import { useRenameCounterparty } from "@/hooks/use-receivables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { CounterpartyInput } from "./counterparty-input";
import type { Counterparty } from "@/types/api";

export interface RenameCounterpartyDialogProps {
  /** The counterparty to rename; null while closed. */
  counterparty: Counterparty | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Rename a counterparty across all their receivables. Picking (or typing) the
 * name of an existing counterparty merges the two.
 */
export function RenameCounterpartyDialog({
  counterparty,
  open,
  onOpenChange,
}: RenameCounterpartyDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {counterparty && (
          <RenameForm
            counterparty={counterparty}
            onDone={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function RenameForm({
  counterparty,
  onDone,
}: {
  counterparty: Counterparty;
  onDone: () => void;
}) {
  const [name, setName] = useState(counterparty.name);
  const rename = useRenameCounterparty();

  const normalized = name.split(/\s+/).filter(Boolean).join(" ");
  const unchanged = normalized === counterparty.name;
  const valid = normalized.length > 0 && normalized.length <= 100;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!valid || unchanged) return;
    rename.mutate(
      { from_key: counterparty.key, to_name: normalized },
      { onSuccess: onDone }
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>Rename or merge</DialogTitle>
        <DialogDescription>
          Change how &quot;{counterparty.name}&quot; appears on every item. To
          merge duplicates, enter or pick the name you want to keep.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="rename-counterparty">New name</Label>
        <CounterpartyInput
          id="rename-counterparty"
          value={name}
          onChange={setName}
          excludeKeys={[counterparty.key]}
          disabled={rename.isPending}
        />
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onDone}
          disabled={rename.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={rename.isPending || !valid || unchanged}>
          {rename.isPending ? "Saving..." : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}
