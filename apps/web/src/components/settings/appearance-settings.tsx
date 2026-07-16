"use client";

import { Check } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DitherFill } from "@/components/charts/dither-fill";
import {
  useChartTheme,
  type ChartTheme,
} from "@/providers/chart-theme-provider";
import { cn } from "@/lib/utils";

const OPTIONS: { value: ChartTheme; title: string; description: string }[] = [
  {
    value: "clean",
    title: "Clean",
    description: "Flat, minimal vector charts. The default look.",
  },
  {
    value: "dither",
    title: "Dither",
    description: "Ordered-dither pixel charts with Geist Pixel accents.",
  },
];

// Illustrative mini bar chart so each option previews its own texture,
// independent of the currently active theme.
const PREVIEW_HEIGHTS = [42, 72, 54, 92, 66];

function ChartStylePreview({ variant }: { variant: ChartTheme }) {
  return (
    <div className="flex h-16 w-full items-end gap-1.5 rounded-md bg-muted/50 p-2">
      {PREVIEW_HEIGHTS.map((h, i) => (
        <div
          key={i}
          className={cn(
            "relative flex-1 overflow-hidden rounded-sm",
            variant === "clean" && "bg-primary"
          )}
          style={{ height: `${h}%` }}
        >
          {variant === "dither" && <DitherFill color="green" />}
        </div>
      ))}
    </div>
  );
}

export function AppearanceSettings() {
  const { chartTheme, setChartTheme } = useChartTheme();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Chart style</CardTitle>
        <CardDescription>
          How charts and data visualizations are rendered across the app. This
          is separate from the light/dark color mode.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 sm:grid-cols-2">
        {OPTIONS.map((opt) => {
          const active = chartTheme === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => setChartTheme(opt.value)}
              aria-pressed={active}
              className={cn(
                "relative flex flex-col gap-3 rounded-lg border p-4 text-left transition-colors",
                active
                  ? "border-primary ring-1 ring-primary"
                  : "border-border hover:border-primary/40"
              )}
            >
              {active && (
                <span className="absolute right-3 top-3 flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground">
                  <Check className="size-3.5" />
                </span>
              )}
              <ChartStylePreview variant={opt.value} />
              <div>
                <p className="text-sm font-medium">{opt.title}</p>
                <p className="text-xs text-muted-foreground">
                  {opt.description}
                </p>
              </div>
            </button>
          );
        })}
      </CardContent>
    </Card>
  );
}
