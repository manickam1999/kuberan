"use client";

import { lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import type { ThemedAreaChartProps } from "./themed-area-chart";
import type { ThemedDonutProps } from "./themed-donut";
import type { ThemedSankeyProps } from "./themed-sankey";

// Code-split chart renderers. Recharts, d3-sankey and the dither-kit (with
// motion) are the heaviest client dependencies, so they load in their own
// chunks while the surrounding card renders and its data query runs. Each
// wrapper's fallback is sized from its props so the layout does not shift.

const AreaChartImpl = lazy(() =>
  import("./themed-area-chart").then((m) => ({ default: m.ThemedAreaChart }))
);
const DonutImpl = lazy(() =>
  import("./themed-donut").then((m) => ({ default: m.ThemedDonut }))
);
const SankeyImpl = lazy(() =>
  import("./themed-sankey").then((m) => ({ default: m.ThemedSankey }))
);

/** Lazily loaded {@link ThemedAreaChartProps | ThemedAreaChart}. */
export function LazyThemedAreaChart(props: ThemedAreaChartProps) {
  return (
    <Suspense fallback={<Skeleton className={cn("w-full", props.className)} />}>
      <AreaChartImpl {...props} />
    </Suspense>
  );
}

/** Lazily loaded {@link ThemedDonutProps | ThemedDonut}. */
export function LazyThemedDonut(props: ThemedDonutProps) {
  const size = props.size ?? 150;
  return (
    <Suspense
      fallback={
        <Skeleton
          className="shrink-0 rounded-full"
          style={{ width: size, height: size }}
        />
      }
    >
      <DonutImpl {...props} />
    </Suspense>
  );
}

/** Lazily loaded {@link ThemedSankeyProps | ThemedSankey}. */
export function LazyThemedSankey(props: ThemedSankeyProps) {
  return (
    <Suspense
      fallback={
        <Skeleton
          className={cn("w-full", props.className)}
          style={{ height: props.height ?? 360 }}
        />
      }
    >
      <SankeyImpl {...props} />
    </Suspense>
  );
}
