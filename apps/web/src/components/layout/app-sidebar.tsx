"use client";

import { RingMark } from "@/components/brand/ring-mark";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Search, Settings } from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from "@/components/ui/sidebar";
import { StatusChip } from "@/components/layout/status-chip";
import { useAuth } from "@/hooks/use-auth";
import { useReceivablesSummary } from "@/hooks/use-receivables";
import {
  MAIN_NAV,
  SIDEBAR_GROUPS,
  isNavActive,
  type NavItem,
} from "@/lib/nav";

function getUserInitials(firstName: string, lastName: string): string {
  const first = firstName?.[0] ?? "";
  const last = lastName?.[0] ?? "";
  return (first + last).toUpperCase() || "U";
}

/** Whole units with thousands separators, no currency symbol (badge-sized). */
function badgeAmount(cents: number): string {
  return Math.round(cents / 100).toLocaleString("en-MY");
}

interface AppSidebarProps {
  onOpenCommandPalette?: () => void;
}

export function AppSidebar({ onOpenCommandPalette }: AppSidebarProps) {
  const pathname = usePathname();
  const { isMobile, setOpenMobile } = useSidebar();
  const { user, hideBalances } = useAuth();
  const { data: receivables } = useReceivablesSummary();

  const handleLinkClick = () => {
    if (isMobile) {
      setOpenMobile(false);
    }
  };

  const owed = receivables?.total_outstanding ?? 0;

  const renderItem = (item: NavItem) => (
    <SidebarMenuItem key={item.href}>
      <SidebarMenuButton
        asChild
        isActive={isNavActive(pathname, item)}
        tooltip={item.title}
        className="data-[active=true]:font-medium [&>svg]:data-[active=true]:text-primary"
      >
        <Link href={item.href} onClick={handleLinkClick}>
          <item.icon />
          <span>{item.title}</span>
        </Link>
      </SidebarMenuButton>
      {item.href === "/receivables" && owed > 0 && (
        <SidebarMenuBadge className="text-muted-foreground font-normal tabular-nums">
          {hideBalances ? "•••" : badgeAmount(owed)}
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  );

  const userName = user
    ? [user.first_name, user.last_name].filter(Boolean).join(" ") || user.email
    : "Account";

  return (
    <Sidebar>
      <SidebarHeader className="gap-3">
        <Link
          href="/"
          onClick={handleLinkClick}
          className="flex items-center gap-2.5 px-2 py-2"
        >
          <RingMark className="size-9" />
          <div className="flex flex-col group-data-[collapsible=icon]:hidden">
            <span className="font-display text-[22px] leading-none">
              Kuberan
            </span>
            <span className="text-[11px] leading-tight text-muted-foreground">
              Personal finance
            </span>
          </div>
        </Link>
        <button
          type="button"
          onClick={() => {
            handleLinkClick();
            onOpenCommandPalette?.();
          }}
          className="text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-ring/50 flex h-9 w-full items-center gap-2 rounded-[9px] border px-2.5 text-[13px] transition-colors outline-none focus-visible:ring-[3px]"
        >
          <Search className="size-3.5 shrink-0" aria-hidden />
          <span>Search</span>
          <kbd className="text-muted-foreground ml-auto hidden rounded border px-1.5 font-mono text-[10.5px] md:inline">
            ⌘K
          </kbd>
        </button>
      </SidebarHeader>

      <SidebarContent className="scrollbar-slim">
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>{MAIN_NAV.map(renderItem)}</SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        {SIDEBAR_GROUPS.map((group) => (
          <SidebarGroup key={group.label}>
            <SidebarGroupLabel className="font-mono text-[10px] font-medium tracking-[0.1em] text-muted-foreground uppercase">
              {group.label}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>{group.items.map(renderItem)}</SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter className="gap-2.5">
        <StatusChip />
        <Link
          href="/settings"
          onClick={handleLinkClick}
          aria-current={pathname.startsWith("/settings") ? "page" : undefined}
          className="text-sidebar-foreground hover:bg-sidebar-accent focus-visible:ring-ring/50 aria-[current=page]:bg-sidebar-accent flex items-center gap-2.5 rounded-lg px-2 py-1.5 transition-colors outline-none focus-visible:ring-[3px]"
        >
          <Avatar size="sm" className="ring-2 ring-primary/25">
            <AvatarFallback className="bg-primary/15 text-primary text-xs font-semibold">
              {user ? getUserInitials(user.first_name, user.last_name) : "U"}
            </AvatarFallback>
          </Avatar>
          <span className="grid min-w-0 flex-1 leading-tight">
            <b className="text-foreground truncate text-[13px] font-medium">
              {userName}
            </b>
            <small className="text-muted-foreground text-[11px]">Settings</small>
          </span>
          <Settings className="text-muted-foreground size-3.5" aria-hidden />
        </Link>
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  );
}
