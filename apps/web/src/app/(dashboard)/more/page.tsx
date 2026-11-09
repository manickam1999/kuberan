"use client";

import Link from "next/link";
import {
  ChevronRight,
  LayoutList,
  LogOut,
  Smartphone,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Tile } from "@/components/ui/tile";
import { useSidebar } from "@/components/ui/sidebar";
import { StatusChip } from "@/components/layout/status-chip";
import { useAuth } from "@/hooks/use-auth";
import { useInstallPrompt } from "@/hooks/use-install-prompt";
import { useReceivablesSummary } from "@/hooks/use-receivables";
import { formatMoney } from "@/lib/format";
import { MORE_NAV } from "@/lib/nav";

function InstallCard() {
  const { state, install } = useInstallPrompt();
  if (state === null || state === "standalone") return null;

  return (
    <Card className="flex-row items-center gap-3 p-4">
      <Tile tone="accent" size="lg">
        <Smartphone />
      </Tile>
      <div className="grid min-w-0 flex-1 gap-0.5">
        <b className="text-sm font-medium">Add Kuberan to your Home Screen</b>
        <span className="text-muted-foreground text-xs">
          {state === "ios"
            ? "Opens full screen with no browser bar. In Safari tap Share, then Add to Home Screen."
            : state === "promptable"
              ? "Opens full screen with no browser bar, like an app."
              : "Opens full screen with no browser bar. Use your browser menu, then Install app or Add to Home Screen."}
        </span>
      </div>
      {state === "promptable" && (
        <Button size="sm" onClick={() => void install()}>
          Install
        </Button>
      )}
    </Card>
  );
}

/**
 * The phone hub for everything the four main tabs don't reach: the remaining
 * destinations, Settings, host status, log out, "All pages" (the full sidebar
 * sheet) and Add to Home Screen.
 */
export default function MorePage() {
  const { user, logout, hideBalances } = useAuth();
  const { setOpenMobile } = useSidebar();
  const { data: receivables } = useReceivablesSummary();

  const owed = receivables?.total_outstanding ?? 0;
  const detail = (href: string): string => {
    if (href === "/receivables" && owed > 0 && receivables) {
      return hideBalances ? "•••" : formatMoney(owed, receivables.currency);
    }
    return "";
  };

  const initials =
    ((user?.first_name?.[0] ?? "") + (user?.last_name?.[0] ?? "")).toUpperCase() ||
    "U";

  return (
    <div className="mx-auto grid max-w-xl gap-4">
      <h1 className="font-display text-3xl leading-tight">More</h1>

      <Card className="gap-0 overflow-hidden p-0">
        {MORE_NAV.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className="hover:bg-accent focus-visible:bg-accent flex min-h-14 items-center gap-3 border-b px-4 py-2.5 outline-none last:border-b-0"
          >
            <Tile tone="neutral">
              <item.icon />
            </Tile>
            <span className="flex-1 text-sm font-medium">{item.title}</span>
            <span className="text-muted-foreground text-xs tabular-nums">
              {detail(item.href)}
            </span>
            <ChevronRight className="text-muted-foreground size-4" aria-hidden />
          </Link>
        ))}
      </Card>

      <Card className="gap-0 overflow-hidden p-0">
        <Link
          href="/settings"
          className="hover:bg-accent focus-visible:bg-accent flex min-h-14 items-center gap-3 border-b px-4 py-2.5 outline-none"
        >
          <Avatar size="sm" className="ring-2 ring-primary/25">
            <AvatarFallback className="bg-primary/15 text-primary text-xs font-semibold">
              {initials}
            </AvatarFallback>
          </Avatar>
          <span className="grid min-w-0 flex-1">
            <span className="text-sm font-medium">Settings</span>
            <span className="text-muted-foreground truncate text-xs">
              Profile, appearance, Telegram, apps, export
            </span>
          </span>
          <ChevronRight className="text-muted-foreground size-4" aria-hidden />
        </Link>
        <button
          type="button"
          onClick={() => setOpenMobile(true)}
          className="hover:bg-accent focus-visible:bg-accent flex min-h-14 items-center gap-3 border-b px-4 py-2.5 text-left outline-none"
        >
          <Tile tone="neutral">
            <LayoutList />
          </Tile>
          <span className="flex-1 text-sm font-medium">All pages</span>
          <ChevronRight className="text-muted-foreground size-4" aria-hidden />
        </button>
        <button
          type="button"
          onClick={logout}
          className="hover:bg-accent focus-visible:bg-accent flex min-h-14 items-center gap-3 px-4 py-2.5 text-left outline-none"
        >
          <Tile tone="neutral">
            <LogOut />
          </Tile>
          <span className="flex-1 text-sm font-medium">Log out</span>
        </button>
      </Card>

      <StatusChip wide />

      <InstallCard />
    </div>
  );
}
