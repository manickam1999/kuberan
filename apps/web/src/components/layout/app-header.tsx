"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Eye, EyeOff, LogOut, Plus, Search } from "lucide-react";
import { PolarisMark } from "@/components/brand/polaris-mark";
import { HEADER_SLOT_ID } from "@/components/layout/header-slot";
import { RefreshIndicator } from "@/components/layout/refresh-indicator";
import { useAuth } from "@/hooks/use-auth";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { pageTitle } from "@/lib/nav";

function getUserInitials(firstName: string, lastName: string): string {
  const first = firstName?.[0] ?? "";
  const last = lastName?.[0] ?? "";
  return (first + last).toUpperCase() || "U";
}

interface AppHeaderProps {
  onOpenCommandPalette?: () => void;
  /** Opens the add-transaction dialog ("＋ New", `N`). */
  onNew?: () => void;
}

/**
 * Desktop: sidebar trigger, an optional month-switcher slot (`HeaderSlot`),
 * search, hide balances, "＋ New" and the account menu. Phones: the mark and
 * page title with search and hide balances (navigation is the tab bar).
 * A thin bar on the bottom edge shows background refetches.
 */
export function AppHeader({ onOpenCommandPalette, onNew }: AppHeaderProps) {
  const { user, logout, hideBalances, toggleHideBalances } = useAuth();
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-20 flex h-[calc(52px+env(safe-area-inset-top))] shrink-0 items-center gap-1 border-b border-border/60 bg-background/80 pt-[env(safe-area-inset-top)] pr-2.5 pl-3.5 backdrop-blur-md supports-[backdrop-filter]:bg-background/70 md:h-[calc(58px+env(safe-area-inset-top))] md:gap-2 md:px-4">
      <SidebarTrigger className="-ml-1 hidden text-muted-foreground md:inline-flex" />
      <Separator orientation="vertical" className="mr-1 hidden h-5 md:block" />

      <Link
        href="/"
        aria-label="Kuberan home"
        className="mr-1.5 flex items-center md:hidden"
      >
        <PolarisMark className="size-7" />
      </Link>
      <span className="flex-1 truncate text-[15px] font-semibold md:hidden">
        {pageTitle(pathname)}
      </span>

      <div id={HEADER_SLOT_ID} className="hidden items-center md:flex" />
      <div className="hidden flex-1 md:block" />

      <Button
        variant="outline"
        className="relative hidden h-9 w-9 p-0 text-muted-foreground md:inline-flex xl:w-72 xl:justify-start xl:gap-2 xl:px-3 xl:py-2 xl:pr-12 xl:font-normal"
        onClick={onOpenCommandPalette}
      >
        <Search className="size-4 shrink-0" aria-hidden="true" />
        <span className="hidden min-w-0 flex-1 truncate text-left xl:block">
          Search or jump to…
        </span>
        <span className="sr-only">Search</span>
        <kbd className="pointer-events-none absolute top-1/2 right-2 hidden h-5 -translate-y-1/2 select-none items-center gap-0.5 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium xl:flex">
          <span className="text-xs">⌘</span>K
        </kbd>
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className="text-muted-foreground md:hidden"
        onClick={onOpenCommandPalette}
      >
        <Search className="size-[18px]" aria-hidden="true" />
        <span className="sr-only">Search</span>
      </Button>
      <Button
        variant="ghost"
        size="icon"
        onClick={toggleHideBalances}
        aria-pressed={hideBalances}
      >
        {hideBalances ? (
          <EyeOff className="size-4" />
        ) : (
          <Eye className="size-4" />
        )}
        <span className="sr-only">
          {hideBalances ? "Show balances" : "Hide balances"}
        </span>
      </Button>
      <Button className="hidden md:inline-flex" onClick={onNew}>
        <Plus aria-hidden />
        New
        <kbd className="bg-primary-foreground/15 ml-0.5 rounded px-1 font-mono text-[10px]">
          N
        </kbd>
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger className="hidden items-center gap-2 rounded-full py-1 pl-1 pr-2.5 transition-colors outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/50 md:flex">
          <Avatar size="sm" className="ring-2 ring-primary/25">
            <AvatarFallback className="bg-primary/15 text-primary text-xs font-semibold">
              {user
                ? getUserInitials(user.first_name, user.last_name)
                : "U"}
            </AvatarFallback>
          </Avatar>
          <span className="hidden text-sm font-medium lg:inline">
            {user
              ? [user.first_name, user.last_name].filter(Boolean).join(" ") ||
                user.email
              : "User"}
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuLabel>{user?.email ?? "Account"}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={logout}>
            <LogOut />
            Log out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <RefreshIndicator />
    </header>
  );
}
