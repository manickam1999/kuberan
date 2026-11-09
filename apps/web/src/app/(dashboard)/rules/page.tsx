"use client";

import { Fragment, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Reorder, useDragControls } from "motion/react";
import {
  ArrowDown,
  ArrowRight,
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
import { useBaseCurrency } from "@/hooks/use-profile";
import { conditionParts } from "@/lib/rule-format";
import { parseRulePrefill, type RulePrefill } from "@/lib/rule-prefill";
import { Button } from "@/components/ui/button";
import { Pill } from "@/components/ui/pill";
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
import { cn } from "@/lib/utils";
import type { TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

function RulesPageContent() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [createOpen, setCreateOpen] = useState(false);
  const [prefill, setPrefill] = useState<RulePrefill | null>(null);
  const [editRule, setEditRule] = useState<TransactionRule | null>(null);
  const [deleteRule, setDeleteRule] = useState<TransactionRule | null>(null);
  const [applyRule, setApplyRule] = useState<TransactionRule | null>(null);

  const { data: rules, isLoading, isError, refetch } = useRules();
  const reorder = useReorderRules();
  const { data: accountData } = useAccounts({ page_size: 100 });

  // Deep link from Transaction detail:
  // /rules?new=1&description=<text>&category_id=<id>&type=<income|expense>
  // Open the editor prefilled once, then drop the params so a refresh or the
  // back button doesn't reopen it.
  useEffect(() => {
    const seed = parseRulePrefill(searchParams);
    if (!seed) return;
    setPrefill(seed);
    setCreateOpen(true);
    router.replace(pathname, { scroll: false });
  }, [searchParams, router, pathname]);

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

  function openCreate() {
    setPrefill(null);
    setCreateOpen(true);
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Rules
          </h1>
          <p className="text-muted-foreground text-sm">
            New transactions are categorised automatically. Rules run top to
            bottom and the first match wins.
          </p>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          New rule
        </Button>
      </div>

      {isLoading ? (
        <div className="bg-card divide-y overflow-hidden rounded-xl border">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex items-center gap-3 px-4 py-3.5">
              <Skeleton className="hidden size-5 md:block" />
              <Skeleton className="size-[22px] rounded-md" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-3/4" />
              </div>
              <Skeleton className="hidden h-6 w-24 sm:block" />
              <Skeleton className="h-5 w-9 rounded-full" />
            </div>
          ))}
        </div>
      ) : isError ? (
        <div
          role="alert"
          className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center"
        >
          <p className="text-sm">Couldn&apos;t load your rules.</p>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            Try again
          </Button>
        </div>
      ) : ordered.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No rules yet</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Create a rule to categorise matching transactions automatically.
          </p>
          <Button className="mt-4" size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            Create rule
          </Button>
        </div>
      ) : (
        <div className="bg-card overflow-hidden rounded-xl border">
          <Reorder.Group
            as="div"
            axis="y"
            values={ordered}
            onReorder={setOrdered}
            className="divide-y"
          >
            {ordered.map((rule, index) => (
              <RuleRow
                key={rule.id}
                rule={rule}
                position={index + 1}
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
        </div>
      )}

      <RuleDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        initialConditions={prefill?.conditions}
        initialCategoryId={prefill?.categoryId}
      />
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

export default function RulesPage() {
  return (
    <Suspense fallback={null}>
      <RulesPageContent />
    </Suspense>
  );
}

interface RuleRowProps {
  rule: TransactionRule;
  position: number;
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
  position,
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
  const baseCurrency = useBaseCurrency();

  const target = rule.actions?.[0]?.category;

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
      className="group bg-card hover:bg-accent/40 relative flex items-center gap-3 px-3 py-3.5 transition-colors md:px-4"
    >
      <button
        type="button"
        aria-label="Drag to reorder"
        onPointerDown={(e) => dragControls.start(e)}
        className="text-muted-foreground/60 hover:text-foreground hidden size-6 shrink-0 cursor-grab touch-none items-center justify-center rounded-md transition-colors active:cursor-grabbing md:flex"
      >
        <GripVertical className="size-4" />
      </button>

      <span
        aria-hidden
        className="bg-muted text-muted-foreground grid size-[22px] shrink-0 place-items-center rounded-md text-[11px] tabular-nums"
      >
        {position}
      </span>

      <button
        type="button"
        onClick={onEdit}
        className={cn(
          "focus-visible:ring-ring/50 min-w-0 flex-1 rounded text-left outline-none focus-visible:ring-[3px]",
          !rule.is_active && "opacity-60"
        )}
      >
        <span className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{rule.name}</span>
          {!rule.is_active && (
            <span className="text-muted-foreground text-xs">Paused</span>
          )}
        </span>
        <span className="text-muted-foreground mt-0.5 block text-[12.5px] leading-snug">
          {rule.conditions.map((c, i) => {
            const { lead, value } = conditionParts(c, accountName, baseCurrency);
            return (
              <Fragment key={c.id ?? i}>
                {i > 0 && " and "}
                {lead}{" "}
                <b className="text-foreground font-mono text-[11.5px] font-medium break-words">
                  {value}
                </b>
              </Fragment>
            );
          })}
          {target && (
            <span className="sm:hidden">
              {" "}
              <ArrowRight className="inline size-3" aria-hidden />
              <span className="sr-only">sets category</span> {target.name}
            </span>
          )}
        </span>
      </button>

      <span className="text-muted-foreground hidden shrink-0 items-center gap-1.5 sm:flex">
        <ArrowRight className="size-3.5" aria-hidden />
        <span className="sr-only">sets category</span>
        <Pill color={target?.color || undefined}>
          {target?.name ?? "a category"}
        </Pill>
      </span>

      <Switch
        checked={rule.is_active}
        onCheckedChange={toggleActive}
        disabled={updateRule.isPending}
        aria-label={rule.is_active ? "Disable rule" : "Enable rule"}
      />

      <div className="transition-opacity focus-within:opacity-100 data-[state=open]:opacity-100 md:opacity-0 md:group-hover:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label="Rule actions"
            >
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
