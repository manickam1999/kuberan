"use client";

import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import {
  useCategories,
  useCreateCategory,
  useUpdateCategory,
} from "@/hooks/use-categories";
import { isValidCategoryIcon } from "@/lib/category-icons";
import { CategoryIcon } from "@/components/categories/category-icon";
import { CategoryIconPicker } from "@/components/categories/category-icon-picker";
import { Button } from "@/components/ui/button";
import { ColorPalette } from "@/components/ui/color-palette";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Segmented } from "@/components/ui/segmented";
import { Tile } from "@/components/ui/tile";
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
import type { Category, CategoryType } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "SELF_PARENT_CATEGORY") {
      return "A category cannot be its own parent";
    }
    if (error.code === "CATEGORY_NOT_FOUND") return "Category not found";
    if (
      error.code === "INVALID_INPUT" &&
      (error.message.toLowerCase().includes("already exists") ||
        error.message.toLowerCase().includes("duplicate"))
    ) {
      return "A category with this name already exists";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

// Pill look (ui/pill.tsx) on a real button; Pill's asChild can't wrap its colour dot.
const PILL_BUTTON =
  "bg-elevated text-muted-foreground hover:border-input hover:text-foreground focus-visible:ring-ring/50 inline-flex h-6 cursor-pointer items-center gap-1.5 rounded-[7px] border px-2 text-xs whitespace-nowrap outline-none transition-colors focus-visible:ring-[3px]";

export interface CategoryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The category to edit; omit to create a new one. */
  category?: Category | null;
  /** Create mode: preselected type (the page's current tab). */
  initialType?: CategoryType;
  /** Create mode: preselected parent. */
  initialParentId?: string;
  /** Edit mode: shows a Delete button that hands off to the caller. */
  onDelete?: (category: Category) => void;
  /** Edit mode: "+ Add" under Subcategories opens a new category under this parent. */
  onAddSubcategory?: (parent: Category) => void;
  /** Edit mode: open another category (a subcategory pill). */
  onOpenCategory?: (category: Category) => void;
}

/**
 * Create or edit a category (prototype `category-edit`): a live tile preview
 * next to the name, the curated icon picker, the colour presets, type, parent
 * and (when editing a top-level category) its subcategories.
 */
export function CategoryDialog({
  open,
  onOpenChange,
  category,
  initialType = "expense",
  initialParentId,
  onDelete,
  onAddSubcategory,
  onOpenCategory,
}: CategoryDialogProps) {
  const isEdit = !!category;
  const [name, setName] = useState("");
  const [type, setType] = useState<CategoryType>(initialType);
  const [description, setDescription] = useState("");
  const [icon, setIcon] = useState("");
  const [color, setColor] = useState("");
  const [parentId, setParentId] = useState("");
  const [error, setError] = useState("");

  const createCategory = useCreateCategory();
  const updateCategory = useUpdateCategory(category?.id ?? "");
  const isSubmitting = createCategory.isPending || updateCategory.isPending;

  // Potential parents are top-level categories of the same type.
  const { data: sameType } = useCategories(
    open ? { type, page_size: 100 } : undefined
  );
  const parentOptions = (sameType?.data ?? [])
    .filter((c) => !c.parent_id && c.id !== category?.id)
    .sort((a, b) => a.name.localeCompare(b.name));
  const children = isEdit
    ? (sameType?.data ?? []).filter((c) => c.parent_id === category?.id)
    : [];
  // A category that has children can't itself become a child (one level only).
  const canHaveParent = !isEdit || children.length === 0;

  useEffect(() => {
    if (!open) return;
    if (category) {
      setName(category.name);
      setType(category.type);
      setDescription(category.description ?? "");
      setIcon(category.icon ?? "");
      setColor(category.color ?? "");
      setParentId(category.parent_id ? String(category.parent_id) : "");
    } else {
      setName("");
      setType(initialType);
      setDescription("");
      setIcon("");
      setColor("");
      setParentId(initialParentId ?? "");
    }
    setError("");
  }, [open, category, initialType, initialParentId]);

  function handleTypeChange(next: CategoryType) {
    setType(next);
    setParentId("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required");
      return;
    }
    if (trimmedName.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (description.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }
    if (icon !== (category?.icon ?? "") && !isValidCategoryIcon(icon)) {
      setError("Icon must be a single emoji or one of the icons shown");
      return;
    }
    const newParentId = parentId && parentId !== "none" ? parentId : null;

    if (category) {
      const payload: Record<string, unknown> = {};
      if (trimmedName !== category.name) payload.name = trimmedName;
      if (description.trim() !== (category.description ?? "")) {
        payload.description = description.trim();
      }
      if (icon.trim() !== (category.icon ?? "")) payload.icon = icon.trim();
      if (color !== (category.color ?? "")) payload.color = color;
      if (newParentId !== (category.parent_id ?? null)) {
        payload.parent_id = newParentId;
      }
      if (Object.keys(payload).length === 0) {
        onOpenChange(false);
        return;
      }
      updateCategory.mutate(payload, {
        onSuccess: (updated) => {
          toast.success(`Category "${updated.name}" updated`);
          onOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      });
      return;
    }

    createCategory.mutate(
      {
        name: trimmedName,
        type,
        description: description.trim() || undefined,
        icon: icon.trim() || undefined,
        color: color || undefined,
        parent_id: newParentId ?? undefined,
      },
      {
        onSuccess: (created) => {
          toast.success(`Category "${created.name}" created`);
          onOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92dvh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{category ? category.name : "New category"}</DialogTitle>
          <DialogDescription className="sr-only">
            {isEdit
              ? "Edit this category."
              : "Add a category to organise your transactions."}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div
            role="alert"
            className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex items-end gap-3">
            <Tile
              size="xl"
              color={color || undefined}
              tone="neutral"
              aria-hidden
            >
              <CategoryIcon icon={icon} name={name || "?"} className="size-5" />
            </Tile>
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <Label htmlFor="cat-name">Name</Label>
              <Input
                id="cat-name"
                placeholder="e.g. Groceries"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={isSubmitting}
                maxLength={100}
              />
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="cat-icon">Icon</Label>
            <CategoryIconPicker
              id="cat-icon"
              value={icon}
              onChange={setIcon}
              disabled={isSubmitting}
            />
            <p className="text-muted-foreground text-xs">
              Pick from the set so it renders the same in the app, Telegram and
              Claude.
            </p>
          </div>

          <div className="flex flex-col gap-2">
            <Label>Colour</Label>
            <ColorPalette
              value={color}
              onChange={setColor}
              disabled={isSubmitting}
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label>Type</Label>
              <Segmented
                full
                aria-label="Type"
                value={type}
                onValueChange={handleTypeChange}
                options={[
                  { value: "expense", label: "Expense", disabled: isEdit },
                  { value: "income", label: "Income", disabled: isEdit },
                ]}
              />
              {isEdit && (
                <p className="text-muted-foreground text-xs">
                  The type can&apos;t change once a category exists.
                </p>
              )}
            </div>
            {canHaveParent && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="cat-parent">Parent</Label>
                <Select
                  value={parentId || "none"}
                  onValueChange={setParentId}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="cat-parent" className="w-full">
                    <SelectValue placeholder="None (top level)" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">None (top level)</SelectItem>
                    {parentOptions.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="cat-description">Description</Label>
            <Input
              id="cat-description"
              placeholder="Optional"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isSubmitting}
              maxLength={500}
            />
          </div>

          {category && !category.parent_id && (
            <div className="bg-background flex flex-col gap-2 rounded-xl border p-3">
              <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
                Subcategories
              </span>
              <div className="flex flex-wrap items-center gap-2">
                {children.length === 0 && (
                  <span className="text-muted-foreground text-xs">None yet.</span>
                )}
                {children.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => onOpenCategory?.(c)}
                    className={PILL_BUTTON}
                  >
                    {c.color && (
                      <i
                        aria-hidden
                        className="size-[7px] shrink-0 rounded-full"
                        style={{ backgroundColor: c.color }}
                      />
                    )}
                    {c.name}
                  </button>
                ))}
                {onAddSubcategory && (
                  <button
                    type="button"
                    onClick={() => onAddSubcategory(category)}
                    className={PILL_BUTTON}
                  >
                    <Plus className="size-3" aria-hidden />
                    Add
                  </button>
                )}
              </div>
            </div>
          )}

          <DialogFooter className="sm:items-center">
            {category && onDelete && (
              <Button
                type="button"
                variant="ghost"
                className="text-destructive hover:text-destructive sm:mr-auto"
                onClick={() => onDelete(category)}
                disabled={isSubmitting}
              >
                <Trash2 className="size-4" />
                Delete
              </Button>
            )}
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving…" : isEdit ? "Save" : "Create category"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
