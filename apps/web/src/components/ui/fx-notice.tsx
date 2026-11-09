"use client";

import { AlertTriangle } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { formatMoney } from "@/lib/format";
import { cn } from "@/lib/utils";

/** An amount left out of a converted total. Accepts API shapes using `amount` or `value`. */
export interface FxNoticeItem {
  currency: string;
  amount?: number;
  value?: number;
}

export interface FxNoticeProps {
  /** Response `fx_missing`: some amounts had no exchange rate and were left out. */
  fxMissing?: boolean;
  /** Response `unconverted` (analytics: `{currency, amount}`, portfolio: `{currency, value}`). */
  unconverted?: FxNoticeItem[] | null;
  /** Response `fx_stale`: an exchange rate used is out of date. */
  fxStale?: boolean;
  /** `banner` (default) is a bordered box; `inline` is a small muted line. */
  variant?: "banner" | "inline";
  className?: string;
}

/**
 * Note for responses with `fx_missing`, `unconverted` or `fx_stale`:
 * "Excludes US$120.00 — no exchange rate". Renders nothing when there is
 * nothing to report. Amounts are masked per `hide_balances`.
 */
export function FxNotice({
  fxMissing,
  unconverted,
  fxStale,
  variant = "banner",
  className,
}: FxNoticeProps) {
  const { hideBalances } = useAuth();
  const items = (unconverted ?? []).filter(
    (u) => (u.amount ?? u.value ?? 0) !== 0
  );

  const messages: string[] = [];
  if (items.length > 0) {
    const list = items
      .map((u) => formatMoney(u.amount ?? u.value ?? 0, u.currency, hideBalances))
      .join(", ");
    messages.push(`Excludes ${list} — no exchange rate`);
  } else if (fxMissing) {
    messages.push("Some amounts are excluded — no exchange rate");
  }
  if (fxStale) messages.push("Some exchange rates are out of date");

  if (messages.length === 0) return null;

  return (
    <div
      role="status"
      className={cn(
        "flex items-start gap-2 text-xs",
        variant === "banner"
          ? "rounded-md border border-warning/30 bg-warning-muted px-3 py-2 text-warning"
          : "text-muted-foreground",
        className
      )}
    >
      <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      <span>{messages.join(". ")}</span>
    </div>
  );
}
