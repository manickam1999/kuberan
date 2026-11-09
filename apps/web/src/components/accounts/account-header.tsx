"use client";

import Link from "next/link";
import { ArrowLeft, Loader2, Pencil, Pin } from "lucide-react";
import { useUpdateAccount } from "@/hooks/use-accounts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { Account } from "@/types/models";

/**
 * Top of every account detail screen: back link, name, eyebrow line, status
 * and the actions shared by all types (pin, edit) plus type-specific ones.
 */
export function AccountHeader({
  account,
  eyebrow,
  onEdit,
  actions,
}: {
  account: Account;
  /** Muted line above the name, e.g. "Credit card · 21.9% APR". */
  eyebrow: string;
  onEdit: () => void;
  /** Type-specific actions, shown after pin and edit. */
  actions?: React.ReactNode;
}) {
  const update = useUpdateAccount(account.id);
  return (
    <div className="space-y-3">
      <Button asChild variant="ghost" size="sm" className="-ml-2">
        <Link href="/accounts">
          <ArrowLeft className="size-4" />
          Accounts
        </Link>
      </Button>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {eyebrow}
          </span>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="font-display text-3xl leading-tight break-words">
              {account.name}
            </h1>
            {!account.is_active && <Badge tone="neutral">Inactive</Badge>}
          </div>
          {account.description && (
            <p className="text-muted-foreground mt-1 text-sm">
              {account.description}
            </p>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={update.isPending}
            aria-pressed={account.is_pinned}
            onClick={() => update.mutate({ is_pinned: !account.is_pinned })}
          >
            {update.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Pin
                className={`size-4 ${account.is_pinned ? "fill-primary" : ""}`}
              />
            )}
            {account.is_pinned ? "Unpin" : "Pin"}
          </Button>
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Pencil className="size-4" />
            Edit
          </Button>
          {actions}
        </div>
      </div>
    </div>
  );
}
