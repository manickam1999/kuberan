"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Wallet,
  ArrowLeftRight,
  Tag,
  PiggyBank,
  Database,
  TrendingUp,
  BarChart3,
  Settings,
  type LucideIcon,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from "@/components/ui/sidebar";

interface NavItem {
  title: string;
  href: string;
  icon: LucideIcon;
}

interface NavSection {
  label: string;
  items: NavItem[];
}

const navSections: NavSection[] = [
  {
    label: "Overview",
    items: [{ title: "Dashboard", href: "/", icon: LayoutDashboard }],
  },
  {
    label: "Money",
    items: [
      { title: "Accounts", href: "/accounts", icon: Wallet },
      { title: "Transactions", href: "/transactions", icon: ArrowLeftRight },
      { title: "Categories", href: "/categories", icon: Tag },
      { title: "Budgets", href: "/budgets", icon: PiggyBank },
      { title: "Analytics", href: "/analytics", icon: BarChart3 },
    ],
  },
  {
    label: "Investing",
    items: [
      { title: "Investments", href: "/investments", icon: TrendingUp },
      { title: "Securities", href: "/securities", icon: Database },
    ],
  },
];

function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

export function AppSidebar() {
  const pathname = usePathname();
  const { isMobile, setOpenMobile } = useSidebar();

  const handleLinkClick = () => {
    if (isMobile) {
      setOpenMobile(false);
    }
  };

  return (
    <Sidebar>
      <SidebarHeader>
        <Link
          href="/"
          onClick={handleLinkClick}
          className="flex items-center gap-2.5 px-2 py-2"
        >
          <div className="relative flex size-9 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-emerald-600 font-semibold text-primary-foreground shadow-sm shadow-primary/30">
            <span className="text-base leading-none">K</span>
          </div>
          <div className="flex flex-col group-data-[collapsible=icon]:hidden">
            <span className="font-semibold leading-tight tracking-tight">
              Kuberan
            </span>
            <span className="text-[11px] leading-tight text-muted-foreground">
              Personal finance
            </span>
          </div>
        </Link>
      </SidebarHeader>

      <SidebarContent className="scrollbar-slim">
        {navSections.map((section) => (
          <SidebarGroup key={section.label}>
            <SidebarGroupLabel className="text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground/70">
              {section.label}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {section.items.map((item) => (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      asChild
                      isActive={isActive(pathname, item.href)}
                      tooltip={item.title}
                      className="data-[active=true]:font-medium data-[active=true]:shadow-sm"
                    >
                      <Link href={item.href} onClick={handleLinkClick}>
                        <item.icon />
                        <span>{item.title}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              isActive={isActive(pathname, "/settings")}
              tooltip="Settings"
            >
              <Link href="/settings" onClick={handleLinkClick}>
                <Settings />
                <span>Settings</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  );
}
