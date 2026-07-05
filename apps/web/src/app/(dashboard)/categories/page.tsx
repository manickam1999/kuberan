"use client";

import { useState } from "react";
import { Pencil, Plus, Trash2, CornerDownRight } from "lucide-react";
import { useCategories } from "@/hooks/use-categories";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent } from "@/components/ui/card";
import { CreateCategoryDialog } from "@/components/categories/create-category-dialog";
import { EditCategoryDialog } from "@/components/categories/edit-category-dialog";
import { DeleteCategoryDialog } from "@/components/categories/delete-category-dialog";
import type { Category, CategoryType } from "@/types/models";

type OrderedCategory = Category & { isChild: boolean };

/** Sort flat category list into tree order: parents first, children after. */
function treeOrder(categories: Category[]): OrderedCategory[] {
  const result: OrderedCategory[] = [];
  const childrenMap = new Map<string, Category[]>();
  const topLevel: Category[] = [];

  for (const cat of categories) {
    if (cat.parent_id && categories.some((c) => c.id === cat.parent_id)) {
      const siblings = childrenMap.get(cat.parent_id) ?? [];
      siblings.push(cat);
      childrenMap.set(cat.parent_id, siblings);
    } else {
      topLevel.push(cat);
    }
  }

  for (const parent of topLevel) {
    result.push({ ...parent, isChild: false });
    for (const child of childrenMap.get(parent.id) ?? []) {
      result.push({ ...child, isChild: true });
    }
  }
  return result;
}

function CategoryRow({
  category,
  onEdit,
  onDelete,
}: {
  category: OrderedCategory;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-accent/40">
      <div className={`flex min-w-0 flex-1 items-center gap-3 ${category.isChild ? "pl-6" : ""}`}>
        {category.isChild && (
          <CornerDownRight className="size-3.5 shrink-0 text-muted-foreground/50" />
        )}
        <span
          className="flex size-8 shrink-0 items-center justify-center rounded-lg text-sm"
          style={{
            backgroundColor: category.color
              ? `${category.color}22`
              : "var(--muted)",
            color: category.color || "var(--muted-foreground)",
          }}
        >
          {category.icon || category.name.charAt(0).toUpperCase()}
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{category.name}</p>
          {category.description && (
            <p className="truncate text-xs text-muted-foreground">
              {category.description}
            </p>
          )}
        </div>
      </div>

      <Badge variant={category.type === "income" ? "positive" : "negative"}>
        {category.type === "income" ? "Income" : "Expense"}
      </Badge>

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

export default function CategoriesPage() {
  const [typeFilter, setTypeFilter] = useState("all");
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [editCategory, setEditCategory] = useState<Category | null>(null);
  const [deleteCategory, setDeleteCategory] = useState<Category | null>(null);

  const filterType =
    typeFilter === "all" ? undefined : (typeFilter as CategoryType);
  const { data, isLoading } = useCategories({ page, type: filterType });

  const categories = data?.data ?? [];
  const totalPages = data?.total_pages ?? 1;
  const currentPage = data?.page ?? 1;
  const ordered = treeOrder(categories);

  const handleTypeChange = (value: string) => {
    setTypeFilter(value);
    setPage(1);
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Categories</h1>
          <p className="text-sm text-muted-foreground">
            Organize transactions into income and expense groups
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          New category
        </Button>
      </div>

      <Tabs value={typeFilter} onValueChange={handleTypeChange}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="income">Income</TabsTrigger>
          <TabsTrigger value="expense">Expense</TabsTrigger>
        </TabsList>
      </Tabs>

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : categories.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No categories yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Create your first category to organize your transactions.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create category
          </Button>
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          <div className="divide-y divide-border/50">
            {ordered.map((cat) => (
              <CategoryRow
                key={cat.id}
                category={cat}
                onEdit={() => setEditCategory(cat)}
                onDelete={() => setDeleteCategory(cat)}
              />
            ))}
          </div>
        </Card>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Page {currentPage} of {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={currentPage <= 1}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage >= totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}

      <CreateCategoryDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditCategoryDialog
        open={!!editCategory}
        onOpenChange={(open) => {
          if (!open) setEditCategory(null);
        }}
        category={editCategory}
      />
      <DeleteCategoryDialog
        open={!!deleteCategory}
        onOpenChange={(open) => {
          if (!open) setDeleteCategory(null);
        }}
        category={deleteCategory}
      />
    </div>
  );
}
