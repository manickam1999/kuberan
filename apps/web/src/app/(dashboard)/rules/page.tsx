"use client";

import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useRules, useReorderRules, useUpdateRule } from "@/hooks/use-rules";
import { useAccounts } from "@/hooks/use-accounts";
import { summarizeCondition, summarizeRuleTarget } from "@/lib/rule-format";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { RuleDialog } from "@/components/rules/rule-dialog";
import { DeleteRuleDialog } from "@/components/rules/delete-rule-dialog";
import type { TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

export default function RulesPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const [editRule, setEditRule] = useState<TransactionRule | null>(null);
  const [deleteRule, setDeleteRule] = useState<TransactionRule | null>(null);

  const { data: rules, isLoading } = useRules();
  const reorder = useReorderRules();
  const { data: accountData } = useAccounts({ page_size: 100 });

  const accountName = useMemo(() => {
    const map = new Map((accountData?.data ?? []).map((a) => [a.id, a.name]));
    return (id: string) => map.get(id) ?? "Unknown account";
  }, [accountData]);

  const list = rules ?? [];

  function move(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= list.length) return;
    const ids = list.map((r) => r.id);
    [ids[index], ids[target]] = [ids[target], ids[index]];
    reorder.mutate(ids, {
      onError: (err) => toast.error(getErrorMessage(err)),
    });
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Rules</h1>
          <p className="text-sm text-muted-foreground">
            Auto-categorize new transactions. Rules run top to bottom; the first
            match wins.
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          New rule
        </Button>
      </div>

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : list.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No rules yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Create a rule to categorize matching transactions automatically.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create rule
          </Button>
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          <div className="divide-y divide-border/50">
            {list.map((rule, index) => (
              <RuleRow
                key={rule.id}
                rule={rule}
                accountName={accountName}
                isFirst={index === 0}
                isLast={index === list.length - 1}
                reordering={reorder.isPending}
                onMoveUp={() => move(index, -1)}
                onMoveDown={() => move(index, 1)}
                onEdit={() => setEditRule(rule)}
                onDelete={() => setDeleteRule(rule)}
              />
            ))}
          </div>
        </Card>
      )}

      <RuleDialog open={createOpen} onOpenChange={setCreateOpen} />
      <RuleDialog
        open={!!editRule}
        onOpenChange={(open) => {
          if (!open) setEditRule(null);
        }}
        rule={editRule}
      />
      <DeleteRuleDialog
        open={!!deleteRule}
        onOpenChange={(open) => {
          if (!open) setDeleteRule(null);
        }}
        rule={deleteRule}
      />
    </div>
  );
}

interface RuleRowProps {
  rule: TransactionRule;
  accountName: (id: string) => string;
  isFirst: boolean;
  isLast: boolean;
  reordering: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

function RuleRow({
  rule,
  accountName,
  isFirst,
  isLast,
  reordering,
  onMoveUp,
  onMoveDown,
  onEdit,
  onDelete,
}: RuleRowProps) {
  const updateRule = useUpdateRule(rule.id);

  const summary = rule.conditions
    .map((c) => summarizeCondition(c, accountName))
    .join(" and ");

  function toggleActive(next: boolean) {
    updateRule.mutate(
      { is_active: next },
      { onError: (err) => toast.error(getErrorMessage(err)) }
    );
  }

  return (
    <div className="group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-accent/40">
      <div className="flex flex-col">
        <Button
          variant="ghost"
          size="icon"
          className="size-6"
          onClick={onMoveUp}
          disabled={isFirst || reordering}
          aria-label="Move up"
        >
          <ArrowUp className="size-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-6"
          onClick={onMoveDown}
          disabled={isLast || reordering}
          aria-label="Move down"
        >
          <ArrowDown className="size-3.5" />
        </Button>
      </div>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{rule.name}</p>
        <p className="truncate text-xs text-muted-foreground">
          {summary}
          <span className="text-foreground"> → {summarizeRuleTarget(rule)}</span>
        </p>
      </div>

      <Switch
        checked={rule.is_active}
        onCheckedChange={toggleActive}
        disabled={updateRule.isPending}
        aria-label={rule.is_active ? "Disable rule" : "Enable rule"}
      />

      <div className="flex gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
        <Button variant="ghost" size="icon" className="size-8" onClick={onEdit}>
          <Pencil className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-8 text-muted-foreground hover:text-destructive"
          onClick={onDelete}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}
