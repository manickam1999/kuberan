"use client";

import { useEffect } from "react";
import { useTheme } from "next-themes";
import { useAuth } from "@/hooks/use-auth";
import { resolvePalette, useServerDefaultPalette } from "@/hooks/use-appearance";
import { applyAndPersistPalette } from "@/lib/palette-apply";
import { useChartTheme } from "@/providers/chart-theme-provider";

/**
 * Keeps this device's look in step with the account. Renders nothing.
 *
 * - Signed in: the user's palette, colour mode and chart style (from the
 *   session user) are applied, so settings follow the user across devices.
 * - Signed out: the server's default palette (the public `GET /appearance`)
 *   is applied, so the sign-in, register and consent pages match the server.
 * - While the session is loading, the palette the init script applied from
 *   the cookie stays, which avoids a flash.
 *
 * Each effect runs on a preference change only, so picking a palette or mode
 * in the UI (which applies it at once) is not fought by a stale value.
 */
export function AppearanceSync() {
  const { user, status } = useAuth();
  const { setTheme } = useTheme();
  const { setChartTheme } = useChartTheme();
  const { data: serverDefault } = useServerDefaultPalette();

  const palette =
    status === "authenticated"
      ? resolvePalette(user, serverDefault)
      : status === "unauthenticated"
        ? (serverDefault ?? null)
        : null;
  const mode = status === "authenticated" ? user?.appearance?.mode : undefined;
  const chartStyle =
    status === "authenticated" ? user?.appearance?.chart_style : undefined;

  useEffect(() => {
    if (palette) applyAndPersistPalette(palette);
  }, [palette]);

  useEffect(() => {
    if (mode) setTheme(mode);
  }, [mode, setTheme]);

  useEffect(() => {
    if (chartStyle) setChartTheme(chartStyle);
  }, [chartStyle, setChartTheme]);

  return null;
}
