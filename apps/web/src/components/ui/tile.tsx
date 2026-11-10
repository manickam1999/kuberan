import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const tileVariants = cva(
  "grid shrink-0 place-items-center text-(--tc) bg-[color-mix(in_oklab,var(--tc)_15%,transparent)] [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      size: {
        sm: "size-6 rounded-[7px] [&_svg:not([class*='size-'])]:size-3.5",
        md: "size-8 rounded-control",
        lg: "size-[42px] rounded-xl [&_svg:not([class*='size-'])]:size-5",
        xl: "size-[46px] rounded-[13px] [&_svg:not([class*='size-'])]:size-5",
      },
    },
    defaultVariants: { size: "md" },
  }
)

const TONES = {
  accent: "var(--primary)",
  pos: "var(--positive)",
  neg: "var(--negative)",
  warn: "var(--warning)",
  info: "var(--info)",
  investment: "var(--investment)",
  neutral: "var(--muted-foreground)",
} as const

export type TileTone = keyof typeof TONES

export interface TileProps
  extends React.ComponentProps<"span">,
    VariantProps<typeof tileVariants> {
  /** A palette tone, used when `color` is not given. */
  tone?: TileTone
  /** Any CSS colour (e.g. a stored category hex); overrides `tone`. */
  color?: string
}

/** Tinted icon square: the icon in a colour on a 15% wash of that colour. */
export function Tile({
  tone = "accent",
  color,
  size,
  className,
  style,
  ...props
}: TileProps) {
  return (
    <span
      data-slot="tile"
      className={cn(tileVariants({ size }), className)}
      style={{ ["--tc" as string]: color ?? TONES[tone], ...style }}
      {...props}
    />
  )
}

export { tileVariants }
