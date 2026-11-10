"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Ellipsis, House, List, Plus, Wallet } from "lucide-react";
import { activeTab, type TabKey } from "@/lib/nav";
import { cn } from "@/lib/utils";

interface MobileTabBarProps {
  /** Opens the add-transaction dialog (the centre ＋ tab). */
  onNew?: () => void;
}

const TABS = [
  { key: "home", label: "Home", href: "/", icon: House },
  { key: "activity", label: "Activity", href: "/transactions", icon: List },
  { key: "wealth", label: "Wealth", href: "/accounts", icon: Wallet },
  { key: "more", label: "More", href: "/more", icon: Ellipsis },
] as const satisfies readonly {
  key: TabKey;
  label: string;
  href: string;
  icon: typeof House;
}[];

function Tab({
  tab,
  current,
}: {
  tab: (typeof TABS)[number];
  current: TabKey;
}) {
  const on = current === tab.key;
  return (
    <Link
      href={tab.href}
      aria-current={on ? "page" : undefined}
      className={cn(
        "focus-visible:ring-ring/50 flex min-h-11 flex-col items-center justify-center gap-0.5 rounded-lg py-1 text-[10.5px] outline-none focus-visible:ring-[3px]",
        on ? "text-primary" : "text-muted-foreground"
      )}
    >
      <tab.icon className="size-[21px]" aria-hidden />
      <span>{tab.label}</span>
    </Link>
  );
}

/**
 * Bottom tab bar below `md`: Home, Activity, ＋, Wealth, More. Fixed to the
 * bottom edge; `--tabbar-h` (globals.css, set while this bar is mounted) is
 * what page content pads by so the bar never covers the last item. The
 * safe-area inset comes from `--safe-bottom`, shared with the dialog sheet.
 */
export function MobileTabBar({ onNew }: MobileTabBarProps) {
  const pathname = usePathname();
  const current = activeTab(pathname);

  return (
    <nav
      data-slot="mobile-tab-bar"
      aria-label="Primary"
      className="bg-background/95 fixed inset-x-0 bottom-0 z-30 grid h-(--tabbar-h) grid-cols-5 items-center border-t px-1.5 pt-1.5 pb-(--safe-bottom) backdrop-blur-md md:hidden"
    >
      <Tab tab={TABS[0]} current={current} />
      <Tab tab={TABS[1]} current={current} />
      <button
        type="button"
        onClick={onNew}
        aria-label="Add transaction"
        className="bg-primary text-primary-foreground focus-visible:ring-ring/50 mx-auto grid size-12 place-items-center rounded-2xl shadow-[0_8px_20px_-8px_var(--primary)] outline-none focus-visible:ring-[3px]"
      >
        <Plus className="size-[22px]" strokeWidth={2.2} aria-hidden />
      </button>
      <Tab tab={TABS[2]} current={current} />
      <Tab tab={TABS[3]} current={current} />
    </nav>
  );
}
