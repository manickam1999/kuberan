"use client";

import { useEffect, useMemo, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import {
  useApplyRule,
  useCreateRule,
  useRulePreview,
  useUpdateRule,
} from "@/hooks/use-rules";
import { useAccounts } from "@/hooks/use-accounts";
import { useCategories } from "@/hooks/use-categories";
import {
  FIELD_LABELS,
  OPERATOR_LABELS,
  OPERATORS_BY_FIELD,
  RULE_FIELDS,
  isConditionComplete,
} from "@/lib/rule-format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CurrencyInput } from "@/components/ui/currency-input";
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
import type { RuleConditionInput } from "@/types/api";
import type { RuleField, RuleOperator, TransactionRule } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) return error.message;
  return "An unexpected error occurred";
}

// A working draft mirrors RuleConditionInput but keeps amounts as plain numbers.
type ConditionDraft = {
  field: RuleField;
  operator: RuleOperator;
  value_text: string;
  amount_min: number; // cents
  amount_max: number; // cents
};

function newCondition(): ConditionDraft {
  return {
    field: "description",
    operator: "contains",
    value_text: "",
    amount_min: 0,
    amount_max: 0,
  };
}

// Map persisted rule conditions into editable drafts.
function conditionsToDrafts(rule: TransactionRule): ConditionDraft[] {
  if (!rule.conditions?.length) return [newCondition()];
  return rule.conditions.map((c) => ({
    field: c.field,
    operator: c.operator,
    value_text: c.value_text ?? "",
    amount_min: c.amount_min ?? 0,
    amount_max: c.amount_max ?? 0,
  }));
}

// Map a persisted/public condition input back into an editable draft.
function inputToDraft(c: RuleConditionInput): ConditionDraft {
  return {
    field: c.field,
    operator: c.operator,
    value_text: c.value_text ?? "",
    amount_min: c.amount_min ?? 0,
    amount_max: c.amount_max ?? 0,
  };
}

// Normalize a draft into the wire shape, dropping fields irrelevant to the field/operator.
function draftToInput(d: ConditionDraft): RuleConditionInput {
  if (d.field === "amount") {
    return {
      field: "amount",
      operator: d.operator,
      amount_min:
        d.operator === "gt" || d.operator === "between" ? d.amount_min : null,
      amount_max:
        d.operator === "lt" || d.operator === "between" ? d.amount_max : null,
    };
  }
  return { field: d.field, operator: d.operator, value_text: d.value_text };
}

interface RuleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** When provided, the dialog edits this rule; otherwise it creates a new one. */
  rule?: TransactionRule | null;
  /** Prefill for the "create rule from transaction" flow (PR 3). */
  initialConditions?: RuleConditionInput[];
  initialCategoryId?: string;
}

export function RuleDialog({
  open,
  onOpenChange,
  rule,
  initialConditions,
  initialCategoryId,
}: RuleDialogProps) {
  const isEdit = !!rule;

  const [name, setName] = useState("");
  const [conditions, setConditions] = useState<ConditionDraft[]>([
    newCondition(),
  ]);
  const [categoryId, setCategoryId] = useState("");
  const [error, setError] = useState("");
  const [previewCount, setPreviewCount] = useState<number | null>(null);

  const createRule = useCreateRule();
  const updateRule = useUpdateRule(rule?.id ?? "");
  const applyRule = useApplyRule();
  const preview = useRulePreview();
  const isSubmitting = createRule.isPending || updateRule.isPending;

  const { data: accountData } = useAccounts({ page_size: 100 });
  const accounts = accountData?.data ?? [];
  const { data: categoryData } = useCategories({ page_size: 100 });
  const categories = useMemo(
    () => [...(categoryData?.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [categoryData]
  );

  // Seed the form when opened.
  useEffect(() => {
    if (!open) return;
    if (rule) {
      setName(rule.name);
      setConditions(conditionsToDrafts(rule));
      setCategoryId(rule.actions?.[0]?.category_id ?? "");
    } else {
      setName("");
      setConditions(
        initialConditions?.length
          ? initialConditions.map(inputToDraft)
          : [newCondition()]
      );
      setCategoryId(initialCategoryId ?? "");
    }
    setError("");
    setPreviewCount(null);
  }, [open, rule, initialConditions, initialCategoryId]);

  // Live "matches N existing" preview, debounced, once all conditions are complete.
  const inputs = useMemo(() => conditions.map(draftToInput), [conditions]);
  const allComplete =
    inputs.length > 0 && inputs.every((c) => isConditionComplete(c));
  const inputsKey = JSON.stringify(inputs);

  useEffect(() => {
    if (!open || !allComplete) {
      setPreviewCount(null);
      return;
    }
    const handle = setTimeout(() => {
      preview.mutate(
        { conditions: inputs },
        { onSuccess: (res) => setPreviewCount(res.count) }
      );
    }, 400);
    return () => clearTimeout(handle);
    // preview is a stable mutation object; inputsKey captures the payload.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, allComplete, inputsKey]);

  function updateCondition(index: number, patch: Partial<ConditionDraft>) {
    setConditions((prev) =>
      prev.map((c, i) => (i === index ? { ...c, ...patch } : c))
    );
  }

  function handleFieldChange(index: number, field: RuleField) {
    // Reset operator to the first valid one and clear values.
    updateCondition(index, {
      field,
      operator: OPERATORS_BY_FIELD[field][0],
      value_text: "",
      amount_min: 0,
      amount_max: 0,
    });
  }

  function addCondition() {
    setConditions((prev) => [...prev, newCondition()]);
  }

  function removeCondition(index: number) {
    setConditions((prev) =>
      prev.length > 1 ? prev.filter((_, i) => i !== index) : prev
    );
  }

  function handleClose(next: boolean) {
    onOpenChange(next);
  }

  // After a create, offer to backfill matching existing transactions.
  async function offerBackfill(ruleId: string) {
    try {
      const dry = await applyRule.mutateAsync({
        ruleId,
        scope: "uncategorized",
        dry_run: true,
      });
      if (dry.count > 0) {
        toast.success("Rule created", {
          description: `${dry.count} existing transaction${dry.count === 1 ? "" : "s"} match this rule.`,
          action: {
            label: "Categorize them",
            onClick: () => {
              applyRule
                .mutateAsync({ ruleId, scope: "uncategorized", dry_run: false })
                .then((res) =>
                  toast.success(`Categorized ${res.applied} transaction${res.applied === 1 ? "" : "s"}`)
                )
                .catch((err) => toast.error(getErrorMessage(err)));
            },
          },
        });
      } else {
        toast.success("Rule created");
      }
    } catch {
      toast.success("Rule created");
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required");
      return;
    }
    if (trimmedName.length > 120) {
      setError("Name must be 120 characters or less");
      return;
    }
    if (!inputs.every((c) => isConditionComplete(c))) {
      setError("Every condition needs a complete value");
      return;
    }
    if (!categoryId) {
      setError("Choose a category to assign");
      return;
    }

    const payload = {
      name: trimmedName,
      conditions: inputs,
      actions: [{ action_type: "set_category" as const, category_id: categoryId }],
    };

    if (isEdit && rule) {
      updateRule.mutate(payload, {
        onSuccess: () => {
          toast.success("Rule updated");
          handleClose(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      });
    } else {
      createRule.mutate(payload, {
        onSuccess: (created) => {
          handleClose(false);
          void offerBackfill(created.id);
        },
        onError: (err) => setError(getErrorMessage(err)),
      });
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit rule" : "New rule"}</DialogTitle>
          <DialogDescription>
            When a new income or expense transaction matches all conditions, it is
            categorized automatically.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="rule-name">Name</Label>
            <Input
              id="rule-name"
              placeholder="e.g. Grab rides"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={120}
            />
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label>Conditions (all must match)</Label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={addCondition}
                disabled={isSubmitting}
              >
                <Plus className="size-4" />
                Add
              </Button>
            </div>
            <div className="flex flex-col gap-2">
              {conditions.map((c, i) => (
                <ConditionRow
                  key={i}
                  condition={c}
                  disabled={isSubmitting}
                  removable={conditions.length > 1}
                  accounts={accounts}
                  onFieldChange={(field) => handleFieldChange(i, field)}
                  onChange={(patch) => updateCondition(i, patch)}
                  onRemove={() => removeCondition(i)}
                />
              ))}
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="rule-category">Assign category</Label>
            <Select
              value={categoryId}
              onValueChange={setCategoryId}
              disabled={isSubmitting}
            >
              <SelectTrigger id="rule-category" className="w-full">
                <SelectValue placeholder="Select a category" />
              </SelectTrigger>
              <SelectContent>
                {categories.map((cat) => (
                  <SelectItem key={cat.id} value={cat.id}>
                    {cat.icon ? `${cat.icon} ` : ""}
                    {cat.name}
                    <span className="ml-1 text-xs text-muted-foreground">
                      {cat.type === "income" ? "Income" : "Expense"}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {allComplete && (
            <p className="text-sm text-muted-foreground">
              {preview.isPending
                ? "Checking existing transactions…"
                : previewCount != null
                  ? `Matches ${previewCount} existing transaction${previewCount === 1 ? "" : "s"}.`
                  : ""}
            </p>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting
                ? isEdit
                  ? "Saving…"
                  : "Creating…"
                : isEdit
                  ? "Save rule"
                  : "Create rule"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

interface ConditionRowProps {
  condition: ConditionDraft;
  disabled: boolean;
  removable: boolean;
  accounts: { id: string; name: string }[];
  onFieldChange: (field: RuleField) => void;
  onChange: (patch: Partial<ConditionDraft>) => void;
  onRemove: () => void;
}

function ConditionRow({
  condition,
  disabled,
  removable,
  accounts,
  onFieldChange,
  onChange,
  onRemove,
}: ConditionRowProps) {
  const operators = OPERATORS_BY_FIELD[condition.field];

  return (
    <div className="flex flex-col gap-2 rounded-lg border p-3">
      <div className="flex items-center gap-2">
        <Select
          value={condition.field}
          onValueChange={(v) => onFieldChange(v as RuleField)}
          disabled={disabled}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RULE_FIELDS.map((f) => (
              <SelectItem key={f} value={f}>
                {FIELD_LABELS[f]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={condition.operator}
          onValueChange={(v) => onChange({ operator: v as RuleOperator })}
          disabled={disabled || operators.length === 1}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {operators.map((op) => (
              <SelectItem key={op} value={op}>
                {OPERATOR_LABELS[op]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {removable && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-9 shrink-0 text-muted-foreground hover:text-destructive"
            onClick={onRemove}
            disabled={disabled}
          >
            <Trash2 className="size-4" />
          </Button>
        )}
      </div>

      <ConditionValue
        condition={condition}
        disabled={disabled}
        accounts={accounts}
        onChange={onChange}
      />
    </div>
  );
}

function ConditionValue({
  condition,
  disabled,
  accounts,
  onChange,
}: {
  condition: ConditionDraft;
  disabled: boolean;
  accounts: { id: string; name: string }[];
  onChange: (patch: Partial<ConditionDraft>) => void;
}) {
  if (condition.field === "description") {
    return (
      <Input
        placeholder="e.g. GRAB"
        value={condition.value_text}
        onChange={(e) => onChange({ value_text: e.target.value })}
        disabled={disabled}
        maxLength={500}
      />
    );
  }

  if (condition.field === "type") {
    return (
      <Select
        value={condition.value_text}
        onValueChange={(v) => onChange({ value_text: v })}
        disabled={disabled}
      >
        <SelectTrigger className="w-full">
          <SelectValue placeholder="Select type" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="income">Income</SelectItem>
          <SelectItem value="expense">Expense</SelectItem>
        </SelectContent>
      </Select>
    );
  }

  if (condition.field === "account_id") {
    return (
      <Select
        value={condition.value_text}
        onValueChange={(v) => onChange({ value_text: v })}
        disabled={disabled}
      >
        <SelectTrigger className="w-full">
          <SelectValue placeholder="Select account" />
        </SelectTrigger>
        <SelectContent>
          {accounts.map((a) => (
            <SelectItem key={a.id} value={a.id}>
              {a.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  // amount
  if (condition.operator === "between") {
    return (
      <div className="flex items-center gap-2">
        <CurrencyInput
          value={condition.amount_min}
          onChange={(cents) => onChange({ amount_min: cents })}
          disabled={disabled}
        />
        <span className="text-sm text-muted-foreground">and</span>
        <CurrencyInput
          value={condition.amount_max}
          onChange={(cents) => onChange({ amount_max: cents })}
          disabled={disabled}
        />
      </div>
    );
  }
  return (
    <CurrencyInput
      value={condition.operator === "gt" ? condition.amount_min : condition.amount_max}
      onChange={(cents) =>
        onChange(
          condition.operator === "gt"
            ? { amount_min: cents }
            : { amount_max: cents }
        )
      }
      disabled={disabled}
    />
  );
}
