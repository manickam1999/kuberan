"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import { apiClient } from "@/lib/api-client";
import { useAuth } from "@/hooks/use-auth";
import { profileKeys } from "@/hooks/use-profile";
import { applyAndPersistPalette } from "@/lib/palette-apply";
import {
  appearanceSaver,
  type ConfirmedAppearance,
  type SaveStatus,
} from "@/lib/appearance-saver";
import {
  BUILT_IN_PALETTE,
  isPaletteId,
  type PaletteId,
} from "@/lib/palettes";
import { useChartTheme } from "@/providers/chart-theme-provider";
import type {
  Appearance,
  AppearanceChartStyle,
  AppearanceDensity,
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
 * (palette attribute, next-themes mode, chart style, and the session user so
 * every control shows it as selected) and save it to the account in the
 * background (`appearanceSaver`: quick changes are merged, only the newest
 * save counts, and a failed save puts the last saved look back).
 */
export function useAppearance() {
  const { user, patchUser } = useAuth();
  const queryClient = useQueryClient();
  const { theme, setTheme } = useTheme();
  const { chartTheme, setChartTheme } = useChartTheme();
  const { data: serverDefault } = useServerDefaultPalette();
  const saveStatus = useSyncExternalStore(
    (cb) => appearanceSaver.subscribe(cb),
    () => appearanceSaver.getStatus(),
    () => "idle" as SaveStatus
  );

  const stored = user?.appearance;
  const appearance: Appearance = useMemo(() => stored ?? {}, [stored]);
  const paletteChoice: AppearancePaletteChoice = appearance.palette ?? "default";
  const serverDefaultPalette = serverDefault ?? BUILT_IN_PALETTE;
  const resolvedPalette =
    resolvePalette(user, serverDefault) ?? serverDefaultPalette;

  /** Writes appearance fields into the session user and the profile cache. */
  const showFields = useCallback(
    (fields: { appearance: Appearance; resolved_palette?: string }) => {
      patchUser(fields);
      queryClient.setQueryData<User>(profileKeys.all, (prev) =>
        prev ? { ...prev, ...fields } : prev
      );
    },
    [patchUser, queryClient]
  );

  /** Puts a confirmed state back on screen after a failed save. */
  const restore = useCallback(
    (confirmed: ConfirmedAppearance) => {
      showFields(confirmed);
      const palette = isPaletteId(confirmed.resolved_palette)
        ? confirmed.resolved_palette
        : serverDefaultPalette;
      applyAndPersistPalette(palette, { animate: true });
      setTheme(confirmed.appearance.mode ?? "system");
      setChartTheme(confirmed.appearance.chart_style ?? "clean");
    },
    [serverDefaultPalette, setChartTheme, setTheme, showFields]
  );

  const change = useCallback(
    (patch: Appearance, failMessage: string) => {
      const current: ConfirmedAppearance = {
        appearance,
        resolved_palette: resolvedPalette,
      };
      const nextAppearance = { ...appearance, ...patch };
      const nextResolved =
        patch.palette === undefined
          ? resolvedPalette
          : patch.palette === "default"
            ? serverDefaultPalette
            : patch.palette;
      showFields({ appearance: nextAppearance, resolved_palette: nextResolved });

      appearanceSaver.schedule(patch, current, {
        send: async (body) => {
          const req: UpdateProfileSettingsRequest = { appearance: body };
          const res = await apiClient.patch<ProfileResponse>(
            "/api/v1/profile",
            req
          );
          return {
            appearance: res.user.appearance ?? {},
            resolved_palette: res.user.resolved_palette,
          };
        },
        onSaved: (confirmed) => showFields(confirmed),
        onFailed: (confirmed) => {
          restore(confirmed);
          toast.error(failMessage);
        },
      });
    },
    [appearance, resolvedPalette, restore, serverDefaultPalette, showFields]
  );

  const setPalette = useCallback(
    (choice: AppearancePaletteChoice) => {
      applyAndPersistPalette(
        choice === "default" ? serverDefaultPalette : choice,
        { animate: true }
      );
      change({ palette: choice }, "Could not save your palette");
    },
    [change, serverDefaultPalette]
  );

  const setMode = useCallback(
    (mode: AppearanceMode) => {
      setTheme(mode);
      change({ mode }, "Could not save your colour mode");
    },
    [change, setTheme]
  );

  const setChartStyle = useCallback(
    (style: AppearanceChartStyle) => {
      setChartTheme(style);
      change({ chart_style: style }, "Could not save your chart style");
    },
    [change, setChartTheme]
  );

  return {
    appearance,
    /** Saves `density` (stored only; consumers read `appearance.density`). */
    setDensity: (density: AppearanceDensity) =>
      change({ density }, "Could not save your display setting"),
    /** Saves `show_cents` (stored only; `<Money hideCents>` opts in). */
    setShowCents: (show: boolean) =>
      change({ show_cents: show }, "Could not save your display setting"),
    paletteChoice,
    serverDefaultPalette,
    resolvedPalette,
    mode: (theme as AppearanceMode | undefined) ?? "system",
    chartStyle: chartTheme,
    setPalette,
    setMode,
    setChartStyle,
    /** Background save state, for a quiet "Saving…" / "Saved" note. */
    saveStatus,
    isSaving: saveStatus === "saving",
  };
}
