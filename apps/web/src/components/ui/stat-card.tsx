import * as React from "react";
import Link from "next/link";
import { ArrowUpRight, ArrowDownRight, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

type Tone = "default" | "primary" | "positive" | "negative" | "warning" | "investment";

const toneChip: Record<Tone, string> = {
  default: "bg-muted text-muted-foreground",
  primary: "bg-primary/12 text-primary",
  positive: "bg-positive-muted text-positive",
  negative: "bg-negative-muted text-negative",
  warning: "bg-warning-muted text-warning",
  investment: "bg-investment-muted text-investment",
};

const toneValue: Record<Tone, string> = {
  default: "text-foreground",
  primary: "text-foreground",
  positive: "text-positive",
  negative: "text-negative",
  warning: "text-warning",
  investment: "text-foreground",
};

export interface StatCardProps {
  label: string;
  value: string;
  icon: LucideIcon;
  tone?: Tone;
  /** A delta like { text: "+10.6%", direction: "up" }. */
  delta?: { text: string; direction: "up" | "down" | "flat" };
  /** Small muted line under the value. */
  sub?: React.ReactNode;
  href?: string;
  className?: string;
}

export function StatCard({
  label,
  value,
  icon: Icon,
  tone = "default",
  delta,
  sub,
  href,
  className,
}: StatCardProps) {
  const inner = (
    <div
      className={cn(
        "group relative flex h-full flex-col justify-between overflow-hidden rounded-xl border bg-card p-4 shadow-sm transition-colors sm:p-5",
        href && "hover:border-primary/40 hover:bg-accent/40",
        className
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {label}
        </span>
        <span
          className={cn(
            "flex size-8 shrink-0 items-center justify-center rounded-lg",
            toneChip[tone]
          )}
        >
          <Icon className="size-4" />
        </span>
      </div>
      <div className="mt-3">
        <div
          className={cn(
            "money text-2xl font-semibold leading-tight sm:text-[1.65rem]",
            toneValue[tone]
          )}
        >
          {value}
        </div>
        <div className="mt-1.5 flex items-center gap-2 text-xs">
          {delta && (
            <span
              className={cn(
                "inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 font-medium",
                delta.direction === "up" && "bg-positive-muted text-positive",
                delta.direction === "down" && "bg-negative-muted text-negative",
                delta.direction === "flat" && "bg-muted text-muted-foreground"
              )}
            >
              {delta.direction === "up" && <ArrowUpRight className="size-3" />}
              {delta.direction === "down" && <ArrowDownRight className="size-3" />}
              {delta.text}
            </span>
          )}
          {sub && <span className="text-muted-foreground">{sub}</span>}
        </div>
      </div>
    </div>
  );

  return href ? (
    <Link href={href} className="block h-full">
      {inner}
    </Link>
  ) : (
    inner
  );
}
