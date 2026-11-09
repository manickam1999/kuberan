"use client"

import * as React from "react"
import { ToggleGroup } from "radix-ui"

import { cn } from "@/lib/utils"

export interface SegmentedOption<T extends string> {
  value: T
  label: React.ReactNode
  disabled?: boolean
}

export interface SegmentedProps<T extends string> {
  value: T
  onValueChange: (value: T) => void
  options: readonly SegmentedOption<T>[]
  /** Accessible name of the group. */
  "aria-label"?: string
  /** Stretch to the container width with equal-width segments. */
  full?: boolean
  className?: string
}

/**
 * Segmented control (prototype `.seg`): one choice out of a few. Radix
 * ToggleGroup in single mode, which exposes radiogroup/radio semantics and
 * arrow-key navigation. Clicking the selected segment does not unselect it.
 */
export function Segmented<T extends string>({
  value,
  onValueChange,
  options,
  full,
  className,
  "aria-label": ariaLabel,
}: SegmentedProps<T>) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      onValueChange={(v) => {
        if (v) onValueChange(v as T)
      }}
      aria-label={ariaLabel}
      data-slot="segmented"
      className={cn(
        "bg-background inline-flex gap-0.5 rounded-control border p-0.5",
        full && "flex w-full",
        className
      )}
    >
      {options.map((o) => (
        <ToggleGroup.Item
          key={o.value}
          value={o.value}
          disabled={o.disabled}
          className={cn(
            "text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 data-[state=on]:bg-muted data-[state=on]:text-foreground inline-flex min-h-9 items-center justify-center rounded-[7px] px-2.5 text-xs font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-[3px] disabled:pointer-events-none disabled:opacity-50 md:min-h-0 md:py-1",
            full && "flex-1"
          )}
        >
          {o.label}
        </ToggleGroup.Item>
      ))}
    </ToggleGroup.Root>
  )
}
