export const SETTINGS_TABS = [
  "profile",
  "appearance",
  "telegram",
  "connected-apps",
  "data",
] as const;

export type SettingsTab = (typeof SETTINGS_TABS)[number];

/** The `?tab=` value as a known tab; anything else opens Profile. */
export function parseSettingsTab(
  value: string | null | undefined
): SettingsTab {
  return (SETTINGS_TABS as readonly string[]).includes(value ?? "")
    ? (value as SettingsTab)
    : "profile";
}
