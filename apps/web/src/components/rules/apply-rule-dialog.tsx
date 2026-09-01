"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useApplyRule } from "@/hooks/use-rules";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { RuleApplyScope } from "@/types/api";
import type { TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

interface ApplyRuleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule: TransactionRule | null;
}

// Backfill an existing rule over past transactions. Runs a dry-run to preview the
// count, then commits on confirm. This is the post-creation path to the same
// backfill offered by the create toast.
export function ApplyRuleDialog({
  open,
  onOpenChange,
  rule,
}: ApplyRuleDialogProps) {
  const [scope, setScope] = useState<RuleApplyScope>("uncategorized");
  const [overwrite, setOverwrite] = useState(false);
  const [dryCount, setDryCount] = useState<number | null>(null);
  const [error, setError] = useState("");

  const applyRule = useApplyRule();

  // Reset when (re)opened.
  useEffect(() => {
    if (open) {
      setScope("uncategorized");
      setOverwrite(false);
      setDryCount(null);
      setError("");
    }
  }, [open]);

  // Dry-run preview whenever the rule or options change.
  useEffect(() => {
    if (!open || !rule) return;
    let cancelled = false;
    setDryCount(null);
    applyRule
      .mutateAsync({ ruleId: rule.id, scope, overwrite, dry_run: true })
      .then((res) => {
        if (!cancelled) setDryCount(res.count);
      })
      .catch((err) => {
        if (!cancelled) setError(getErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
    // applyRule is a stable mutation object.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, rule, scope, overwrite]);

  function handleApply() {
    if (!rule) return;
    setError("");
    applyRule.mutate(
      { ruleId: rule.id, scope, overwrite, dry_run: false },
      {
        onSuccess: (res) => {
          toast.success(
            `Categorized ${res.applied} transaction${res.applied === 1 ? "" : "s"}`
          );
          onOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

  const isBusy = applyRule.isPending;
  const nothingToApply = dryCount === 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Apply to existing transactions</DialogTitle>
          <DialogDescription>
            Run &quot;{rule?.name}&quot; against your existing transactions.
            Categories change only; balances are never affected.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="apply-scope">Scope</Label>
            <Select
              value={scope}
              onValueChange={(v) => setScope(v as RuleApplyScope)}
              disabled={isBusy}
            >
              <SelectTrigger id="apply-scope" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="uncategorized">
                  Only uncategorized transactions
                </SelectItem>
                <SelectItem value="all">All matching transactions</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {scope === "all" && (
            <div className="flex items-center justify-between gap-3">
              <div className="flex flex-col">
                <Label htmlFor="apply-overwrite">Overwrite existing categories</Label>
                <span className="text-xs text-muted-foreground">
                  Replace categories on transactions that already have one.
                </span>
              </div>
              <Switch
                id="apply-overwrite"
                checked={overwrite}
                onCheckedChange={setOverwrite}
                disabled={isBusy}
              />
            </div>
          )}

          <p className="text-sm text-muted-foreground">
            {dryCount == null
              ? "Checking…"
              : nothingToApply
                ? "No matching transactions to categorize."
                : `${dryCount} transaction${dryCount === 1 ? "" : "s"} will be categorized.`}
          </p>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isBusy}
          >
            Cancel
          </Button>
          <Button onClick={handleApply} disabled={isBusy || nothingToApply || dryCount == null}>
            {isBusy ? "Applying…" : "Apply"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
