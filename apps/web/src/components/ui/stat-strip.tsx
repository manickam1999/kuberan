import * as React from "react"

import { cn } from "@/lib/utils"

type StatTone = "default" | "pos" | "neg" | "warn"

const toneValue: Record<StatTone, string> = {
  default: "text-foreground",
  pos: "text-positive",
  neg: "text-negative",
  warn: "text-warning",
}

export interface StatStripItem {
  label: string
  /** The figure; pass `<Money>` or a formatted string. */
  value: React.ReactNode
  tone?: StatTone
  /** Small muted line under the value. */
  sub?: React.ReactNode
}

export interface StatStripProps {
  items: readonly StatStripItem[]
  className?: string
}

/**
 * One joined strip of 2 to 6 stats (prototype `.kpis`), replacing a row of
 * StatCards. Cells are separated by hairlines; two columns on phones (an odd
 * last cell spans the row), one row from `md`.
 */
export function StatStrip({ items, className }: StatStripProps) {
  return (
    <div
      data-slot="stat-strip"
      className={cn(
        "bg-border grid grid-cols-2 gap-px overflow-hidden rounded-card border md:[grid-template-columns:repeat(var(--cols),minmax(0,1fr))]",
        className
      )}
      style={{ ["--cols" as string]: items.length }}
    >
      {items.map((item) => (
        <div
          key={item.label}
          className="bg-card grid min-w-0 content-start gap-1 px-3.5 py-3 max-md:last:odd:col-span-2 md:px-[18px] md:py-3.5"
        >
          <span className="text-muted-foreground text-xs">{item.label}</span>
          <span
            className={cn(
              "money text-[21px] leading-[1.1] tracking-tight md:text-2xl",
              toneValue[item.tone ?? "default"]
            )}
          >
            {item.value}
          </span>
          {item.sub ? (
            <span className="text-muted-foreground text-xs">{item.sub}</span>
          ) : null}
        </div>
      ))}
    </div>
  )
}
