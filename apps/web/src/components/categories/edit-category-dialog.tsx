"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useUpdateCategory, useCategories } from "@/hooks/use-categories";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ColorPalette } from "@/components/ui/color-palette";
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
import type { Category } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "SELF_PARENT_CATEGORY") {
      return "A category cannot be its own parent";
    }
    if (error.code === "CATEGORY_NOT_FOUND") {
      return "Category not found";
    }
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

interface EditCategoryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  category: Category | null;
}

export function EditCategoryDialog({
  open,
  onOpenChange,
  category,
}: EditCategoryDialogProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [icon, setIcon] = useState("");
  const [color, setColor] = useState("");
  const [parentId, setParentId] = useState("");
  const [error, setError] = useState("");

  const updateCategory = useUpdateCategory(category?.id ?? "");
  const isSubmitting = updateCategory.isPending;

  // Load potential parents of same type, excluding self
  const { data: parentData } = useCategories(
    category ? { type: category.type, page_size: 100 } : undefined
  );
  const parentOptions = (parentData?.data ?? [])
    .filter((c) => !c.parent_id && c.id !== category?.id)
    .sort((a, b) => a.name.localeCompare(b.name));

  // Sync form state when category changes
  useEffect(() => {
    if (category) {
      setName(category.name);
      setDescription(category.description ?? "");
      setIcon(category.icon ?? "");
      setColor(category.color ?? "");
      setParentId(category.parent_id ? String(category.parent_id) : "");
      setError("");
    }
  }, [category]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!category) return;
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
    if (icon.length > 50) {
      setError("Icon must be 50 characters or less");
      return;
    }

    // Build payload with only changed fields
    const payload: Record<string, unknown> = {};
    if (trimmedName !== category.name) payload.name = trimmedName;
    const newDesc = description.trim();
    if (newDesc !== (category.description ?? "")) payload.description = newDesc;
    const newIcon = icon.trim();
    if (newIcon !== (category.icon ?? "")) payload.icon = newIcon;
    if (color !== (category.color ?? "")) payload.color = color;
    const newParentId = parentId && parentId !== "none" ? parentId : null;
    const origParentId = category.parent_id ?? null;
    if (newParentId !== origParentId) payload.parent_id = newParentId;

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
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Category</DialogTitle>
          <DialogDescription>
            Update category details.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-cat-name">Name</Label>
            <Input
              id="edit-cat-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
              maxLength={100}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label>Type</Label>
            <Badge
              variant={category?.type === "income" ? "default" : "destructive"}
              className="w-fit"
            >
              {category?.type === "income" ? "Income" : "Expense"}
            </Badge>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-cat-description">Description</Label>
            <Input
              id="edit-cat-description"
              placeholder="Optional description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isSubmitting}
              maxLength={500}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-cat-icon">Icon</Label>
            <Input
              id="edit-cat-icon"
              placeholder="e.g. 🛒 or food"
              value={icon}
              onChange={(e) => setIcon(e.target.value)}
              disabled={isSubmitting}
              maxLength={50}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label>Color</Label>
            <ColorPalette
              value={color}
              onChange={setColor}
              disabled={isSubmitting}
            />
          </div>

          {parentOptions.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="edit-cat-parent">Parent Category</Label>
              <Select
                value={parentId || "none"}
                onValueChange={setParentId}
                disabled={isSubmitting}
              >
                <SelectTrigger id="edit-cat-parent" className="w-full">
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">None</SelectItem>
                  {parentOptions.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving..." : "Save Changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
