"use client";

import type { ReactNode } from "react";
import { Pie as RechartsPie, PieChart as RechartsPieChart } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from "@/components/ui/chart";
import { PieChart } from "@/components/dither-kit/pie-chart";
import { Pie } from "@/components/dither-kit/pie";
import { Tooltip } from "@/components/dither-kit/tooltip";
import type { ChartConfig as DitherChartConfig } from "@/components/dither-kit/chart-context";
import type { DitherColor } from "@/components/dither-kit/palette";
import { useChartTheme } from "@/providers/chart-theme-provider";

export type DonutSlice = {
  name: string;
  /** Slice magnitude; must share a unit with `totalForPct`. */
  value: number;
  /** Clean-theme slice colour (any CSS colour). */
  cleanColor: string;
  /** Dither-theme palette hue. */
  ditherColor: DitherColor;
};

export type ThemedDonutProps = {
  data: DonutSlice[];
  /** Square edge length in px. */
  size?: number;
  /** Formats a slice value for the tooltip. */
  valueFormatter: (value: number) => string;
  /** Denominator for the tooltip percentage (same unit as slice `value`). */
  totalForPct: number;
  /** Centered overlay content (e.g. a total). */
  center?: ReactNode;
};

/**
 * Donut/pie that renders the flat Recharts look (`clean`) or the ordered-dither
 * look (`dither`) based on the active chart theme. Colour per slice is chosen
 * from the slice's `cleanColor`/`ditherColor` so both themes stay on-palette.
 * The center overlay is theme-agnostic DOM so it reads identically in both.
 */
export function ThemedDonut({
  data,
  size = 150,
  valueFormatter,
  totalForPct,
  center,
}: ThemedDonutProps) {
  const { chartTheme } = useChartTheme();

  const pct = (value: number) =>
    totalForPct > 0 ? ((value / totalForPct) * 100).toFixed(1) : "0";

  return (
    <div
      className="relative shrink-0"
      style={{ width: size, height: size }}
    >
      {chartTheme === "dither" ? (
        <PieChart
          data={data}
          config={data.reduce<DitherChartConfig>((acc, slice) => {
            acc[slice.name] = { label: slice.name, color: slice.ditherColor };
            return acc;
          }, {})}
          dataKey="value"
          nameKey="name"
          innerRadius={0.78}
          margins={{ top: 14, right: 14, bottom: 14, left: 14 }}
          bloom="low"
          bloomOnHover
        >
          <Pie variant="gradient" />
          <Tooltip
            valueFormatter={(value) =>
              `${valueFormatter(value)} (${pct(value)}%)`
            }
          />
        </PieChart>
      ) : (
        <ChartContainer
          config={
            data.reduce<ChartConfig>((acc, slice) => {
              acc[slice.name] = { label: slice.name };
              return acc;
            }, {}) satisfies ChartConfig
          }
          className="aspect-square size-full"
        >
          <RechartsPieChart>
            <ChartTooltip
              cursor={false}
              content={({ active, payload }) => {
                if (!active || !payload?.length) return null;
                const item = payload[0];
                const value = item.value as number;
                return (
                  <div className="rounded-lg border border-border/50 bg-popover px-3 py-2 text-xs shadow-xl">
                    <div className="flex items-center gap-2">
                      <span
                        className="size-2.5 rounded-[2px]"
                        style={{ backgroundColor: item.payload.fill }}
                      />
                      <span className="font-medium">{item.name}</span>
                    </div>
                    <div className="mt-1 text-muted-foreground">
                      {valueFormatter(value)} ({pct(value)}%)
                    </div>
                  </div>
                );
              }}
            />
            <RechartsPie
              data={data.map((slice) => ({
                name: slice.name,
                value: slice.value,
                fill: slice.cleanColor,
              }))}
              dataKey="value"
              nameKey="name"
              innerRadius={Math.round(size * 0.34)}
              strokeWidth={4}
            />
          </RechartsPieChart>
        </ChartContainer>
      )}
      {center && (
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          {center}
        </div>
      )}
    </div>
  );
}
