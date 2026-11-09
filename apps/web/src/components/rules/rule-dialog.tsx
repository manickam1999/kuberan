"use client";

import { useEffect, useMemo, useState } from "react";
import { CategoryIcon } from "@/components/categories/category-icon";
import { Check, Plus, Trash2 } from "lucide-react";
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
import { useBaseCurrency } from "@/hooks/use-profile";
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
import { Checkbox } from "@/components/ui/checkbox";
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
  const [recategorise, setRecategorise] = useState(false);

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
    setRecategorise(false);
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

  // "Recategorise them too": move every existing match into the rule's category.
  async function recategoriseExisting(ruleId: string, saved: string) {
    try {
      const res = await applyRule.mutateAsync({
        ruleId,
        scope: "all",
        overwrite: true,
        dry_run: false,
      });
      toast.success(saved, {
        description: `Recategorised ${res.applied} transaction${res.applied === 1 ? "" : "s"}.`,
      });
    } catch (err) {
      toast.error(`${saved}, but recategorising failed`, {
        description: getErrorMessage(err),
      });
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    // A rule needs a name; derive one from its first text condition (or the
    // category) when the field is left empty.
    const firstText = conditions.find(
      (c) => c.field === "description" && c.value_text.trim()
    )?.value_text;
    const trimmedName = (
      name.trim() ||
      firstText?.trim() ||
      categories.find((c) => c.id === categoryId)?.name ||
      ""
    );
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
        onSuccess: (updated) => {
          handleClose(false);
          if (recategorise) void recategoriseExisting(updated.id, "Rule updated");
          else toast.success("Rule updated");
        },
        onError: (err) => setError(getErrorMessage(err)),
      });
    } else {
      createRule.mutate(payload, {
        onSuccess: (created) => {
          handleClose(false);
          if (recategorise) void recategoriseExisting(created.id, "Rule created");
          else void offerBackfill(created.id);
        },
        onError: (err) => setError(getErrorMessage(err)),
      });
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-h-[92dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit rule" : "New rule"}</DialogTitle>
          <DialogDescription>
            When a new income or expense transaction matches all conditions, it is
            categorised automatically.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div
            role="alert"
            className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="rule-name">Name</Label>
            <Input
              id="rule-name"
              placeholder="e.g. Petrol (optional)"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={120}
            />
          </div>

          <fieldset className="bg-background flex flex-col gap-2 rounded-xl border p-3">
            <legend className="sr-only">Conditions</legend>
            <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              When all of these match
            </span>
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
            <button
              type="button"
              onClick={addCondition}
              disabled={isSubmitting}
              className="text-primary focus-visible:ring-ring/50 flex w-fit items-center gap-1 rounded text-sm outline-none hover:underline focus-visible:ring-[3px]"
            >
              <Plus className="size-3.5" aria-hidden />
              Add condition
            </button>
          </fieldset>

          <div className="bg-background flex flex-col gap-2 rounded-xl border p-3">
            <Label
              htmlFor="rule-category"
              className="text-muted-foreground text-xs font-medium tracking-wide uppercase"
            >
              Then set category
            </Label>
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
                    {cat.icon && (
                      <CategoryIcon icon={cat.icon} name={cat.name} className="mr-1" />
                    )}
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
            <div
              aria-live="polite"
              className="bg-background flex flex-col gap-2 rounded-xl border p-3 text-sm"
            >
              {preview.isPending && previewCount == null ? (
                <span className="text-muted-foreground">
                  Checking existing transactions…
                </span>
              ) : previewCount != null ? (
                <>
                  <span className="flex items-center gap-2">
                    <Check className="text-positive size-4 shrink-0" aria-hidden />
                    <span>
                      Matches{" "}
                      <b className="font-medium">
                        {previewCount} existing transaction
                        {previewCount === 1 ? "" : "s"}
                      </b>
                      .
                    </span>
                  </span>
                  {previewCount > 0 && categoryId && (
                    <span className="flex items-start gap-2">
                      <Checkbox
                        id="rule-recategorise"
                        checked={recategorise}
                        onCheckedChange={(v) => setRecategorise(v === true)}
                        disabled={isSubmitting}
                        className="mt-0.5"
                      />
                      <Label
                        htmlFor="rule-recategorise"
                        className="flex-col items-start gap-0.5 font-normal"
                      >
                        Recategorise them too
                        <span className="text-muted-foreground text-xs">
                          Replaces their current category when you save.
                        </span>
                      </Label>
                    </span>
                  )}
                </>
              ) : null}
            </div>
          )}

          <DialogFooter className="sm:items-center">
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleClose(false)}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving…" : "Save rule"}
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
    <div className="flex flex-col gap-2 rounded-lg border bg-card p-2.5">
      <div className="flex items-center gap-2">
        <Select
          value={condition.field}
          onValueChange={(v) => onFieldChange(v as RuleField)}
          disabled={disabled}
        >
          <SelectTrigger className="w-full" aria-label="Field">
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
          <SelectTrigger className="w-full" aria-label="Operator">
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
            aria-label="Remove condition"
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
  const baseCurrency = useBaseCurrency();
  if (condition.field === "description") {
    return (
      <Input
        aria-label="Text to match"
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
          currency={baseCurrency}
          value={condition.amount_min}
          onChange={(cents) => onChange({ amount_min: cents })}
          disabled={disabled}
        />
        <span className="text-sm text-muted-foreground">and</span>
        <CurrencyInput
          currency={baseCurrency}
          value={condition.amount_max}
          onChange={(cents) => onChange({ amount_max: cents })}
          disabled={disabled}
        />
      </div>
    );
  }
  return (
    <CurrencyInput
      currency={baseCurrency}
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
