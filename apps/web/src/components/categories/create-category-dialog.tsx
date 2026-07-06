"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { useCreateCategory, useCategories } from "@/hooks/use-categories";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import type { CategoryType } from "@/types/models";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
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

interface CreateCategoryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateCategoryDialog({
  open,
  onOpenChange,
}: CreateCategoryDialogProps) {
  const [name, setName] = useState("");
  const [type, setType] = useState<CategoryType | "">("");
  const [description, setDescription] = useState("");
  const [icon, setIcon] = useState("");
  const [color, setColor] = useState("");
  const [parentId, setParentId] = useState("");
  const [error, setError] = useState("");

  const createCategory = useCreateCategory();
  const isSubmitting = createCategory.isPending;

  // Load potential parents filtered by selected type
  const { data: parentData } = useCategories(
    type ? { type: type as CategoryType, page_size: 100 } : undefined
  );
  const parentOptions = (parentData?.data ?? [])
    .filter((c) => !c.parent_id)
    .sort((a, b) => a.name.localeCompare(b.name));

  function resetForm() {
    setName("");
    setType("");
    setDescription("");
    setIcon("");
    setColor("");
    setParentId("");
    setError("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) resetForm();
    onOpenChange(nextOpen);
  }

  function handleTypeChange(value: string) {
    setType(value as CategoryType);
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
    if (!type) {
      setError("Type is required");
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

    createCategory.mutate(
      {
        name: trimmedName,
        type: type as CategoryType,
        description: description.trim() || undefined,
        icon: icon.trim() || undefined,
        color: color || undefined,
        parent_id: parentId || undefined,
      },
      {
        onSuccess: (category) => {
          toast.success(`Category "${category.name}" created`);
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
          <DialogTitle>Create Category</DialogTitle>
          <DialogDescription>
            Add a new category to organize your transactions.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
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

          <div className="flex flex-col gap-2">
            <Label htmlFor="cat-type">Type</Label>
            <Select
              value={type}
              onValueChange={handleTypeChange}
              disabled={isSubmitting}
            >
              <SelectTrigger id="cat-type" className="w-full">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="income">Income</SelectItem>
                <SelectItem value="expense">Expense</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="cat-description">Description</Label>
            <Input
              id="cat-description"
              placeholder="Optional description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isSubmitting}
              maxLength={500}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="cat-icon">Icon</Label>
            <Input
              id="cat-icon"
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

          {type && parentOptions.length > 0 && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="cat-parent">Parent Category</Label>
              <Select
                value={parentId}
                onValueChange={setParentId}
                disabled={isSubmitting}
              >
                <SelectTrigger id="cat-parent" className="w-full">
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
              {isSubmitting ? "Creating..." : "Create Category"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
