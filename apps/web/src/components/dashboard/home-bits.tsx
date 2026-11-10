"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { Card } from "@/components/ui/card";
import { Tile } from "@/components/ui/tile";
import { CategoryIcon } from "@/components/categories/category-icon";
import { transactionVisual } from "@/lib/domain-visuals";
import { cn } from "@/lib/utils";
import type { Category, TransactionType } from "@/types/models";

/** Small mono caption above a figure (prototype `.eyebrow`). */
export function Eyebrow({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <span
      className={cn(
        "font-mono text-[11px] uppercase tracking-[0.12em] text-muted-foreground",
        className
      )}
    >
      {children}
    </span>
  );
}

/** A Home card: title, optional link or note on the right, then the body. */
export function HomeCard({
  title,
  href,
  linkLabel,
  note,
  className,
  children,
}: {
  title: string;
  href?: string;
  linkLabel?: string;
  note?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Card className={cn("h-full gap-3 px-5", className)}>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {href ? (
          <Link
            href={href}
            className="rounded-sm text-xs text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
          >
            {linkLabel ?? "All"}
          </Link>
        ) : (
          note
        )}
      </div>
      {children}
    </Card>
  );
}

/** The tile for a category (its colour and icon), or the transaction type's icon. */
export function CategoryTile({
  category,
  type,
  size,
}: {
  category?: Pick<Category, "color" | "icon" | "name"> | null;
  type?: TransactionType;
  size?: "sm" | "md";
}) {
  if (category) {
    return (
      <Tile color={category.color} size={size}>
        <CategoryIcon icon={category.icon} name={category.name} />
      </Tile>
    );
  }
  const v = transactionVisual(type ?? "expense");
  const Icon = v.icon;
  return (
    <Tile tone="neutral" size={size}>
      <Icon />
    </Tile>
  );
}
