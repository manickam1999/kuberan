"use client";

import { useId } from "react";
import {
  AreaChart as RechartsAreaChart,
  Area as RechartsArea,
  XAxis as RechartsXAxis,
  YAxis as RechartsYAxis,
  CartesianGrid,
} from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from "@/components/ui/chart";
import { AreaChart } from "@/components/dither-kit/area-chart";
import { Area } from "@/components/dither-kit/area";
import { XAxis } from "@/components/dither-kit/x-axis";
import { YAxis } from "@/components/dither-kit/y-axis";
import { Grid } from "@/components/dither-kit/grid";
import { Tooltip } from "@/components/dither-kit/tooltip";
import type { DitherColor } from "@/components/dither-kit/palette";
import { useChartTheme } from "@/providers/chart-theme-provider";
import { useIsMobile } from "@/hooks/use-mobile";
import { formatCompact } from "@/lib/format";

export type ThemedAreaChartProps = {
  /** Chart points, oldest → newest. `value` is in display units. */
  data: { label: string; value: number }[];
  seriesLabel: string;
  /** Formats a point value for the tooltip (receives the raw `value`). */
  valueFormatter: (value: number) => string;
  /** Height/width classes for the chart region. */
  className?: string;
  /** Dither-theme palette hue. */
  ditherColor?: DitherColor;
  /** Clean-theme stroke/fill colour (any CSS colour). */
  cleanColor?: string;
  /** Y-axis tick formatter for the dither theme (defaults to compact). */
  axisFormatter?: (value: number) => string;
};

/**
 * Time-series area chart that renders the flat Recharts look (`clean`) or the
 * ordered-dither look (`dither`) based on the active chart theme. Call sites
 * pass the same data + formatters regardless of theme.
 */
export function ThemedAreaChart({
  data,
  seriesLabel,
  valueFormatter,
  className,
  ditherColor = "green",
  cleanColor = "var(--chart-1)",
  axisFormatter = formatCompact,
}: ThemedAreaChartProps) {
  const { chartTheme } = useChartTheme();
  const isMobile = useIsMobile();
  const gradientId = `area-${useId().replace(/:/g, "")}`;

  if (chartTheme === "dither") {
    return (
      <div className={className}>
        <AreaChart
          data={data}
          config={{ value: { label: seriesLabel, color: ditherColor } }}
          yDomain="auto"
          bloom="low"
          bloomOnHover
          margins={{ top: 10, right: 10, bottom: 22, left: 44 }}
        >
          <Grid />
          <XAxis dataKey="label" maxTicks={isMobile ? 4 : 7} />
          <YAxis tickFormatter={(v) => axisFormatter(v)} tickCount={4} />
          <Area dataKey="value" variant="gradient" />
          <Tooltip
            labelKey="label"
            valueFormatter={(v) => valueFormatter(v)}
          />
        </AreaChart>
      </div>
    );
  }

  const config = {
    value: { label: seriesLabel, color: cleanColor },
  } satisfies ChartConfig;

  return (
    <ChartContainer config={config} className={className}>
      <RechartsAreaChart accessibilityLayer data={data}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-value)" stopOpacity={0.4} />
            <stop
              offset="95%"
              stopColor="var(--color-value)"
              stopOpacity={0.1}
            />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <RechartsXAxis
          dataKey="label"
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          minTickGap={isMobile ? 50 : 30}
        />
        <RechartsYAxis hide domain={["dataMin - 100", "dataMax + 100"]} />
        <ChartTooltip
          content={({ active, payload, label }) => {
            if (!active || !payload?.length) return null;
            const value = payload[0].value as number;
            return (
              <div className="border-border/50 bg-background rounded-lg border px-3 py-2 text-xs shadow-xl">
                <div className="font-medium">{label}</div>
                <div className="mt-1 font-mono font-medium tabular-nums">
                  {valueFormatter(value)}
                </div>
              </div>
            );
          }}
        />
        <RechartsArea
          type="monotone"
          dataKey="value"
          stroke="var(--color-value)"
          fill={`url(#${gradientId})`}
          strokeWidth={2}
        />
      </RechartsAreaChart>
    </ChartContainer>
  );
}
