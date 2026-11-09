"use client";

import { Check, Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { DitherFill } from "@/components/charts/dither-fill";
import { useAppearance } from "@/hooks/use-appearance";
import { getPalette, PALETTES, type Palette } from "@/lib/palettes";
import type { AppearanceMode, AppearancePaletteChoice } from "@/types/models";
import type { ChartTheme } from "@/providers/chart-theme-provider";
import { cn } from "@/lib/utils";

const MODES: { value: AppearanceMode; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "System", icon: Monitor },
];

const CHART_OPTIONS: {
  value: ChartTheme;
  title: string;
  description: string;
}[] = [
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

/**
 * A miniature screen drawn in the palette's own colours (for the colour mode
 * on screen), independent of the active palette.
 */
function PalettePreview({
  palette,
  dark,
}: {
  palette: Palette;
  dark: boolean;
}) {
  const t = dark ? palette.dark : palette.light;
  return (
    <div
      aria-hidden="true"
      className="flex h-20 w-full flex-col gap-1.5 rounded-md border p-2"
      style={{ backgroundColor: t.ink0, borderColor: t.line2 }}
    >
      <div
        className="flex flex-1 flex-col justify-between rounded-sm p-1.5"
        style={{ backgroundColor: t.ink1, border: `1px solid ${t.line}` }}
      >
        <div className="flex items-center justify-between gap-2">
          <span
            className="h-1.5 w-10 rounded-full"
            style={{ backgroundColor: t.fg }}
          />
          <span
            className="h-2 w-5 rounded-full"
            style={{ backgroundColor: t.accent }}
          />
        </div>
        <div className="flex items-center gap-1.5">
          <span
            className="h-1.5 w-7 rounded-full"
            style={{ backgroundColor: t.fg2 }}
          />
          <span
            className="h-1.5 w-4 rounded-full"
            style={{ backgroundColor: t.pos }}
          />
          <span
            className="h-1.5 w-4 rounded-full"
            style={{ backgroundColor: t.neg }}
          />
        </div>
      </div>
      <div className="flex h-2 gap-0.5">
        {[t.k1, t.k2, t.k3, t.k4, t.k5, t.k6].map((c, i) => (
          <span
            key={i}
            className="flex-1 rounded-[2px]"
            style={{ backgroundColor: c }}
          />
        ))}
      </div>
    </div>
  );
}

function ColourModeCard() {
  const { mode, setMode } = useAppearance();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Colour mode</CardTitle>
        <CardDescription>
          Light, dark, or follow your device. Saved to your account, so it
          follows you to other devices.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div
          className="flex flex-wrap gap-2"
          role="group"
          aria-label="Colour mode"
        >
          {MODES.map(({ value, label, icon: Icon }) => (
            <Button
              key={value}
              type="button"
              variant={mode === value ? "default" : "outline"}
              aria-pressed={mode === value}
              onClick={() => setMode(value)}
            >
              <Icon />
              {label}
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function PaletteCard() {
  const { paletteChoice, serverDefaultPalette, setPalette } = useAppearance();
  const { resolvedTheme } = useTheme();
  const dark = resolvedTheme !== "light";
  const serverPalette = getPalette(serverDefaultPalette);

  const tiles: {
    choice: AppearancePaletteChoice;
    title: string;
    detail: string;
    palette: Palette;
  }[] = [
    {
      choice: "default",
      title: `Server default (${serverPalette.name})`,
      detail: "Follows the palette this server is set to.",
      palette: serverPalette,
    },
    ...PALETTES.map((p) => ({
      choice: p.id as AppearancePaletteChoice,
      title: p.name,
      detail: p.description,
      palette: p,
    })),
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Palette</CardTitle>
        <CardDescription>
          The colours used across the app. Each tile previews its own palette.
          Your choice follows you to every device you sign in on.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {tiles.map((tile) => {
          const active = paletteChoice === tile.choice;
          return (
            <button
              key={tile.choice}
              type="button"
              onClick={() => setPalette(tile.choice)}
              aria-pressed={active}
              className={cn(
                "relative flex flex-col gap-3 rounded-lg border p-3 text-left transition-colors",
                active
                  ? "border-primary ring-1 ring-primary"
                  : "border-border hover:border-primary/40"
              )}
            >
              {active && (
                <span className="absolute right-4 top-4 flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground">
                  <Check className="size-3.5" />
                </span>
              )}
              <PalettePreview palette={tile.palette} dark={dark} />
              <div>
                <p className="text-sm font-medium">{tile.title}</p>
                <p className="line-clamp-2 text-xs text-muted-foreground">
                  {tile.detail}
                </p>
              </div>
            </button>
          );
        })}
      </CardContent>
    </Card>
  );
}

function ChartStyleCard() {
  const { chartStyle, setChartStyle } = useAppearance();

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
        {CHART_OPTIONS.map((opt) => {
          const active = chartStyle === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => setChartStyle(opt.value)}
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

export function AppearanceSettings() {
  return (
    <div className="space-y-4">
      <ColourModeCard />
      <PaletteCard />
      <ChartStyleCard />
    </div>
  );
}
