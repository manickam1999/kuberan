"use client"

import * as React from "react"

import { cn } from "@/lib/utils"

export interface ToggleChipProps
  extends Omit<React.ComponentProps<"button">, "onChange"> {
  pressed: boolean
  onPressedChange?: (pressed: boolean) => void
}

/**
 * Filter chip (prototype `.fchip`): dashed outline when off, solid and lifted
 * when on. A toggle button, so it carries `aria-pressed`.
 */
export function ToggleChip({
  pressed,
  onPressedChange,
  className,
  onClick,
  type = "button",
  ...props
}: ToggleChipProps) {
  return (
    <button
      type={type}
      aria-pressed={pressed}
      data-slot="toggle-chip"
      data-state={pressed ? "on" : "off"}
      onClick={(e) => {
        onClick?.(e)
        if (!e.defaultPrevented) onPressedChange?.(!pressed)
      }}
      className={cn(
        "focus-visible:ring-ring/50 border-input text-muted-foreground inline-flex h-10 shrink-0 items-center gap-1.5 rounded-control border border-dashed px-3 text-[12.5px] whitespace-nowrap transition-colors outline-none focus-visible:ring-[3px] disabled:pointer-events-none disabled:opacity-50 md:h-8",
        "hover:text-foreground data-[state=on]:bg-elevated data-[state=on]:text-foreground data-[state=on]:border-solid",
        "[&_svg]:size-3.5 [&_svg]:shrink-0",
        className
      )}
      {...props}
    />
  )
}
