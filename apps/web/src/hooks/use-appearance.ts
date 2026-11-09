"use client";

import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import { apiClient } from "@/lib/api-client";
import { useAuth } from "@/hooks/use-auth";
import { profileKeys } from "@/hooks/use-profile";
import { applyAndPersistPalette } from "@/lib/palette-apply";
import {
  BUILT_IN_PALETTE,
  isPaletteId,
  type PaletteId,
} from "@/lib/palettes";
import { useChartTheme } from "@/providers/chart-theme-provider";
import type {
  Appearance,
  AppearanceChartStyle,
  AppearanceMode,
  AppearancePaletteChoice,
  User,
} from "@/types/models";
import type {
  AppearanceDefaultResponse,
  ProfileResponse,
  UpdateProfileSettingsRequest,
} from "@/types/api";

export const appearanceKeys = {
  serverDefault: ["appearance", "server-default"] as const,
};

/**
 * The server's default palette (`GET /appearance`, public). The sign-in,
 * register and consent pages read it before any session exists. Falls back to
 * the built-in palette when the API answers with an id this build does not
 * know (a newer server).
 */
export function useServerDefaultPalette() {
  return useQuery({
    queryKey: appearanceKeys.serverDefault,
    queryFn: async (): Promise<PaletteId> => {
      const res = await apiClient.get<AppearanceDefaultResponse>(
        "/api/v1/appearance"
      );
      return isPaletteId(res.default_palette)
        ? res.default_palette
        : BUILT_IN_PALETTE;
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
}

/**
 * The palette to render for a session user: the server-resolved one, else the
 * user's own pick, else the server default.
 */
export function resolvePalette(
  user: Pick<User, "appearance" | "resolved_palette"> | null,
  serverDefault: PaletteId | undefined
): PaletteId | null {
  if (user) {
    if (isPaletteId(user.resolved_palette)) return user.resolved_palette;
    const choice = user.appearance?.palette;
    if (isPaletteId(choice)) return choice;
  }
  return serverDefault ?? null;
}

/**
 * Saves a partial appearance update (`PATCH /profile {appearance}`). The
 * server merges it with the stored preferences and returns the full result,
 * which replaces the session user's appearance and resolved palette.
 */
export function useUpdateAppearance() {
  const queryClient = useQueryClient();
  const { patchUser } = useAuth();

  return useMutation({
    mutationFn: async (patch: Appearance) => {
      const body: UpdateProfileSettingsRequest = { appearance: patch };
      const res = await apiClient.patch<ProfileResponse>(
        "/api/v1/profile",
        body
      );
      return res.user;
    },
    onSuccess: (user) => {
      const fields = {
        appearance: user.appearance ?? {},
        resolved_palette: user.resolved_palette,
      };
      patchUser(fields);
      queryClient.setQueryData<User>(profileKeys.all, (prev) =>
        prev ? { ...prev, ...fields } : prev
      );
    },
  });
}

/**
 * The signed-in user's appearance with setters that apply a change at once
 * (palette attribute, next-themes mode, chart style) and save it to the
 * account. A failed save puts the previous look back.
 */
export function useAppearance() {
  const { user } = useAuth();
  const { theme, setTheme } = useTheme();
  const { chartTheme, setChartTheme } = useChartTheme();
  const { data: serverDefault } = useServerDefaultPalette();
  const update = useUpdateAppearance();
  const { mutate } = update;

  const appearance: Appearance = user?.appearance ?? {};
  const paletteChoice: AppearancePaletteChoice = appearance.palette ?? "default";
  const serverDefaultPalette = serverDefault ?? BUILT_IN_PALETTE;
  const resolvedPalette =
    resolvePalette(user, serverDefault) ?? serverDefaultPalette;

  const setPalette = useCallback(
    (choice: AppearancePaletteChoice) => {
      const previous = resolvedPalette;
      applyAndPersistPalette(
        choice === "default" ? serverDefaultPalette : choice
      );
      mutate(
        { palette: choice },
        {
          onError: () => {
            applyAndPersistPalette(previous);
            toast.error("Could not save your palette");
          },
        }
      );
    },
    [mutate, resolvedPalette, serverDefaultPalette]
  );

  const setMode = useCallback(
    (mode: AppearanceMode) => {
      const previous = theme;
      setTheme(mode);
      mutate(
        { mode },
        {
          onError: () => {
            if (previous) setTheme(previous);
            toast.error("Could not save your colour mode");
          },
        }
      );
    },
    [mutate, setTheme, theme]
  );

  const setChartStyle = useCallback(
    (style: AppearanceChartStyle) => {
      const previous = chartTheme;
      setChartTheme(style);
      mutate(
        { chart_style: style },
        {
          onError: () => {
            setChartTheme(previous);
            toast.error("Could not save your chart style");
          },
        }
      );
    },
    [chartTheme, mutate, setChartTheme]
  );

  return {
    appearance,
    paletteChoice,
    serverDefaultPalette,
    resolvedPalette,
    mode: (theme as AppearanceMode | undefined) ?? "system",
    chartStyle: chartTheme,
    setPalette,
    setMode,
    setChartStyle,
    isSaving: update.isPending,
  };
}
