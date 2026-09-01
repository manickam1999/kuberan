"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Reorder, useDragControls } from "motion/react";
import {
  ArrowDown,
  ArrowUp,
  GripVertical,
  MoreHorizontal,
  Pencil,
  Plus,
  Trash2,
  Wand2,
} from "lucide-react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useRules, useReorderRules, useUpdateRule } from "@/hooks/use-rules";
import { useAccounts } from "@/hooks/use-accounts";
import { summarizeCondition, summarizeRuleTarget } from "@/lib/rule-format";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { RuleDialog } from "@/components/rules/rule-dialog";
import { DeleteRuleDialog } from "@/components/rules/delete-rule-dialog";
import { ApplyRuleDialog } from "@/components/rules/apply-rule-dialog";
import type { TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

export default function RulesPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const [editRule, setEditRule] = useState<TransactionRule | null>(null);
  const [deleteRule, setDeleteRule] = useState<TransactionRule | null>(null);
  const [applyRule, setApplyRule] = useState<TransactionRule | null>(null);

  const { data: rules, isLoading } = useRules();
  const reorder = useReorderRules();
  const { data: accountData } = useAccounts({ page_size: 100 });

  const accountName = useMemo(() => {
    const map = new Map((accountData?.data ?? []).map((a) => [a.id, a.name]));
    return (id: string) => map.get(id) ?? "Unknown account";
  }, [accountData]);

  // Local ordering so drag feels instant; the server call persists on drop.
  const [ordered, setOrdered] = useState<TransactionRule[]>([]);
  useEffect(() => {
    if (rules) setOrdered(rules);
  }, [rules]);

  // Mirror the latest order in a ref so drag-end persists the final order
  // regardless of render timing.
  const orderedRef = useRef<TransactionRule[]>([]);
  orderedRef.current = ordered;

  function persist(next: TransactionRule[]) {
    reorder.mutate(
      next.map((r) => r.id),
      {
        onError: (err) => {
          toast.error(getErrorMessage(err));
          setOrdered(rules ?? []); // revert optimistic order
        },
      }
    );
  }

  // Keyboard-accessible reorder from the row's menu.
  function move(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= ordered.length) return;
    const next = [...ordered];
    [next[index], next[target]] = [next[target], next[index]];
    setOrdered(next);
    persist(next);
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Rules</h1>
          <p className="text-sm text-muted-foreground">
            Auto-categorize new transactions. Rules run top to bottom; the first
            match wins. Drag to reorder.
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
      ) : ordered.length === 0 ? (
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
          <Reorder.Group
            as="div"
            axis="y"
            values={ordered}
            onReorder={setOrdered}
            className="divide-y divide-border/50"
          >
            {ordered.map((rule, index) => (
              <RuleRow
                key={rule.id}
                rule={rule}
                accountName={accountName}
                isFirst={index === 0}
                isLast={index === ordered.length - 1}
                onMoveUp={() => move(index, -1)}
                onMoveDown={() => move(index, 1)}
                onDragEnd={() => persist(orderedRef.current)}
                onEdit={() => setEditRule(rule)}
                onApply={() => setApplyRule(rule)}
                onDelete={() => setDeleteRule(rule)}
              />
            ))}
          </Reorder.Group>
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
      <ApplyRuleDialog
        open={!!applyRule}
        onOpenChange={(open) => {
          if (!open) setApplyRule(null);
        }}
        rule={applyRule}
      />
    </div>
  );
}

interface RuleRowProps {
  rule: TransactionRule;
  accountName: (id: string) => string;
  isFirst: boolean;
  isLast: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onDragEnd: () => void;
  onEdit: () => void;
  onApply: () => void;
  onDelete: () => void;
}

function RuleRow({
  rule,
  accountName,
  isFirst,
  isLast,
  onMoveUp,
  onMoveDown,
  onDragEnd,
  onEdit,
  onApply,
  onDelete,
}: RuleRowProps) {
  const updateRule = useUpdateRule(rule.id);
  const dragControls = useDragControls();

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
    <Reorder.Item
      as="div"
      value={rule}
      dragListener={false}
      dragControls={dragControls}
      onDragEnd={onDragEnd}
      whileDrag={{ scale: 1.01 }}
      className="group relative flex items-center gap-2 bg-card px-4 py-3 transition-colors hover:bg-accent/40"
    >
      <button
        type="button"
        aria-label="Drag to reorder"
        onPointerDown={(e) => dragControls.start(e)}
        className="flex size-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-md text-muted-foreground/60 transition-colors hover:text-foreground active:cursor-grabbing"
      >
        <GripVertical className="size-4" />
      </button>

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

      <div className="opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100 data-[state=open]:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8" aria-label="Rule actions">
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onMoveUp} disabled={isFirst}>
              <ArrowUp className="size-4" />
              Move up
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onMoveDown} disabled={isLast}>
              <ArrowDown className="size-4" />
              Move down
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onEdit}>
              <Pencil className="size-4" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onApply}>
              <Wand2 className="size-4" />
              Apply to existing…
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={onDelete}>
              <Trash2 className="size-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </Reorder.Item>
  );
}
