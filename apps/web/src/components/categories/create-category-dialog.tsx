"use client";

import {
  CategoryDialog,
  type CategoryDialogProps,
} from "@/components/categories/category-dialog";

type CreateCategoryDialogProps = Pick<
  CategoryDialogProps,
  "open" | "onOpenChange" | "initialType" | "initialParentId"
>;

/** Create-mode wrapper around {@link CategoryDialog}. */
export function CreateCategoryDialog(props: CreateCategoryDialogProps) {
  return <CategoryDialog {...props} />;
}
