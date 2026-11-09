"use client";

import { useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { CategoryIcon } from "@/components/categories/category-icon";
import { CategoryDialog } from "@/components/categories/category-dialog";
import { DeleteCategoryDialog } from "@/components/categories/delete-category-dialog";
import { useCategories } from "@/hooks/use-categories";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useCashflow } from "@/hooks/use-transactions";
import { buildCategoryCards } from "@/lib/category-grid";
import { Button } from "@/components/ui/button";
import { FxNotice } from "@/components/ui/fx-notice";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import type { Category, CategoryType } from "@/types/models";

const TYPE_OPTIONS = [
  { value: "expense", label: "Expenses" },
  { value: "income", label: "Income" },
] as const;

export default function CategoriesPage() {
  const [type, setType] = useState<CategoryType>("expense");
  const [createOpen, setCreateOpen] = useState(false);
  const [createParent, setCreateParent] = useState<string | undefined>();
  const [editCategory, setEditCategory] = useState<Category | null>(null);
  const [deleteCategory, setDeleteCategory] = useState<Category | null>(null);

  const month = useActiveMonth();
  const { data, isLoading, isError, refetch } = useCategories({
    type,
    page_size: 100,
  });
  const cashflow = useCashflow(month.fromDate, month.toDate);

  const categories = useMemo(() => data?.data ?? [], [data]);
  const totals = useMemo(() => {
    const items =
      type === "income" ? cashflow.data?.income : cashflow.data?.expenses;
    const map = new Map<string, number>();
    for (const item of items ?? []) {
      if (item.category_id) map.set(item.category_id, item.total);
    }
    return map;
  }, [cashflow.data, type]);
  const cards = useMemo(
    () => buildCategoryCards(categories, totals),
    [categories, totals]
  );
  const baseCurrency = cashflow.data?.currency;
  const showSkeleton = isLoading || month.isLoading;

  function openCreate(parentId?: string) {
    setCreateParent(parentId);
    setCreateOpen(true);
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Categories
          </h1>
          <p className="text-muted-foreground text-sm">
            Spending categories in {month.label}. Subcategories roll up into
            their parent.
          </p>
        </div>
        <Button size="sm" onClick={() => openCreate()}>
          <Plus className="size-4" />
          New category
        </Button>
      </div>

      <Segmented
        aria-label="Category type"
        value={type}
        onValueChange={setType}
        options={TYPE_OPTIONS}
      />

      <FxNotice
        fxMissing={cashflow.data?.fx_missing}
        unconverted={cashflow.data?.unconverted}
      />

      {showSkeleton ? (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(250px,1fr))]">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-[72px] rounded-xl" />
          ))}
        </div>
      ) : isError ? (
        <div
          role="alert"
          className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center"
        >
          <p className="text-sm">Couldn&apos;t load your categories.</p>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            Try again
          </Button>
        </div>
      ) : cards.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">
            No {type === "income" ? "income" : "expense"} categories yet
          </h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Create a category to group your transactions.
          </p>
          <Button className="mt-4" size="sm" onClick={() => openCreate()}>
            <Plus className="size-4" />
            Create category
          </Button>
        </div>
      ) : (
        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-[repeat(auto-fill,minmax(250px,1fr))]">
          {cards.map(({ category, children, total }) => (
            <li key={category.id}>
              <button
                type="button"
                onClick={() => setEditCategory(category)}
                className="bg-card hover:border-input focus-visible:ring-ring/50 grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 rounded-xl border px-4 py-3.5 text-left transition-colors outline-none focus-visible:ring-[3px]"
              >
                <Tile
                  size="lg"
                  color={category.color || undefined}
                  tone="neutral"
                  aria-hidden
                >
                  <CategoryIcon icon={category.icon} name={category.name} />
                </Tile>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    {category.name}
                  </span>
                  {(children.length > 0 || category.description) && (
                    <span className="text-muted-foreground block truncate text-xs">
                      {children.length > 0
                        ? `${children.length} subcategor${children.length === 1 ? "y" : "ies"}`
                        : category.description}
                    </span>
                  )}
                </span>
                {total > 0 && baseCurrency ? (
                  <Money
                    amount={type === "income" ? total : -total}
                    currency={baseCurrency}
                    sign="auto"
                    className="text-sm"
                  />
                ) : (
                  <span className="text-muted-foreground text-sm">
                    <span aria-hidden>—</span>
                    <span className="sr-only">No activity</span>
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}

      <CategoryDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        initialType={type}
        initialParentId={createParent}
      />
      <CategoryDialog
        open={!!editCategory}
        onOpenChange={(open) => {
          if (!open) setEditCategory(null);
        }}
        category={editCategory}
        onDelete={(c) => {
          setEditCategory(null);
          setDeleteCategory(c);
        }}
        onAddSubcategory={(parent) => {
          setEditCategory(null);
          setType(parent.type);
          openCreate(parent.id);
        }}
        onOpenCategory={setEditCategory}
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
