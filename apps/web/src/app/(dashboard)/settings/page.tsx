"use client";

import { Suspense, useEffect } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Download, Link2, Send, Sun, Users } from "lucide-react";
import { AppearanceSettings } from "@/components/settings/appearance-settings";
import { ConnectedAppsSettings } from "@/components/settings/connected-apps-settings";
import { DataSettings } from "@/components/settings/data-settings";
import { ProfileSettings } from "@/components/settings/profile-settings";
import { TelegramSettings } from "@/components/settings/telegram-settings";
import { parseSettingsTab, type SettingsTab } from "@/lib/settings-tabs";
import { cn } from "@/lib/utils";

const NAV: { value: SettingsTab; label: string; icon: typeof Users }[] = [
  { value: "profile", label: "Profile & security", icon: Users },
  { value: "appearance", label: "Appearance", icon: Sun },
  { value: "telegram", label: "Telegram", icon: Send },
  { value: "connected-apps", label: "Connected apps", icon: Link2 },
  { value: "data", label: "Data & export", icon: Download },
];

function SettingsContent() {
  const tab = parseSettingsTab(useSearchParams().get("tab"));

  // On phones the strip scrolls sideways: keep the open tab in view.
  useEffect(() => {
    document
      .querySelector('nav[aria-label="Settings sections"] [aria-current="page"]')
      ?.scrollIntoView({ inline: "center", block: "nearest" });
  }, [tab]);

  return (
    <div className="space-y-5">
      <div className="hidden md:block">
        <h1 className="font-display text-3xl leading-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Your profile, look, bot, connected apps and data
        </p>
      </div>

      <div className="grid gap-5 md:grid-cols-[200px_minmax(0,1fr)] md:items-start">
        {/* A real link per tab, so each is linkable and keyboard reachable.
            On phones the strip scrolls sideways, not the page. */}
        <nav
          aria-label="Settings sections"
          className="-mx-4 flex gap-1 overflow-x-auto px-4 pb-1 md:sticky md:top-20 md:mx-0 md:flex-col md:overflow-visible md:px-0 md:pb-0"
        >
          {NAV.map(({ value, label, icon: Icon }) => (
            <Link
              key={value}
              href={`/settings?tab=${value}`}
              replace
              scroll={false}
              aria-current={tab === value ? "page" : undefined}
              className={cn(
                "flex shrink-0 items-center gap-2 whitespace-nowrap rounded-control px-3 py-2 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                tab === value
                  ? "bg-accent font-medium text-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              )}
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </Link>
          ))}
        </nav>

        <div className="min-w-0">
          {tab === "profile" && <ProfileSettings />}
          {tab === "appearance" && <AppearanceSettings />}
          {tab === "telegram" && <TelegramSettings />}
          {tab === "connected-apps" && <ConnectedAppsSettings />}
          {tab === "data" && <DataSettings />}
        </div>
      </div>
    </div>
  );
}

export default function SettingsPage() {
  return (
    <Suspense fallback={null}>
      <SettingsContent />
    </Suspense>
  );
}
