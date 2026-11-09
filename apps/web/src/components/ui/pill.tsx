import * as React from "react"
import { Slot } from "radix-ui"

import { cn } from "@/lib/utils"

export interface PillProps extends React.ComponentProps<"span"> {
  /** Colour of the leading dot (any CSS colour, e.g. a category hex). */
  color?: string
  /** Render as the child element (a button or link) with hover styling. */
  asChild?: boolean
}

/** Category pill: a 24px rounded chip with a colour dot. */
export function Pill({
  color,
  asChild = false,
  className,
  children,
  ...props
}: PillProps) {
  const Comp = asChild ? Slot.Root : "span"
  return (
    <Comp
      data-slot="pill"
      className={cn(
        "bg-elevated text-muted-foreground inline-flex h-6 items-center gap-1.5 rounded-[7px] border px-2 text-xs whitespace-nowrap",
        asChild &&
          "hover:border-input hover:text-foreground cursor-pointer transition-colors",
        className
      )}
      {...props}
    >
      {color ? (
        <i
          aria-hidden
          className="size-[7px] shrink-0 rounded-full"
          style={{ backgroundColor: color }}
        />
      ) : null}
      {children}
    </Comp>
  )
}
