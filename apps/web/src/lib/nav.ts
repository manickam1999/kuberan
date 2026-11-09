import {
  BarChart3,
  HandCoins,
  House,
  PiggyBank,
  Tag,
  Filter,
  TrendingUp,
  ArrowLeftRight,
  Wallet,
  type LucideIcon,
} from "lucide-react";

/** One destination in the app shell (sidebar, palette, More page). */
export interface NavItem {
  title: string;
  href: string;
  icon: LucideIcon;
  /** Second key of the "G then <key>" go-to shortcut. */
  goKey?: string;
  /** Extra path prefixes that count as being on this destination. */
  also?: readonly string[];
}

export interface NavGroup {
  label: string;
  items: readonly NavItem[];
}

const HOME: NavItem = { title: "Home", href: "/", icon: House, goKey: "h" };
const TRANSACTIONS: NavItem = {
  title: "Transactions",
  href: "/transactions",
  icon: ArrowLeftRight,
  goKey: "t",
};
const ACCOUNTS: NavItem = {
  title: "Accounts",
  href: "/accounts",
  icon: Wallet,
  goKey: "a",
};
const INVESTMENTS: NavItem = {
  title: "Investments",
  href: "/investments",
  icon: TrendingUp,
  goKey: "i",
  // Securities is reached from the Investments tab, not the sidebar.
  also: ["/securities"],
};
const BUDGETS: NavItem = {
  title: "Budgets",
  href: "/budgets",
  icon: PiggyBank,
  goKey: "b",
};
// D5: the label is Insights, the route stays /analytics.
const INSIGHTS: NavItem = {
  title: "Insights",
  href: "/analytics",
  icon: BarChart3,
  goKey: "n",
};
const OWED: NavItem = {
  title: "Owed to me",
  href: "/receivables",
  icon: HandCoins,
};
const CATEGORIES: NavItem = { title: "Categories", href: "/categories", icon: Tag };
const RULES: NavItem = { title: "Rules", href: "/rules", icon: Filter };

/** D4: six main destinations. */
export const MAIN_NAV: readonly NavItem[] = [
  HOME,
  TRANSACTIONS,
  ACCOUNTS,
  INVESTMENTS,
  BUDGETS,
  INSIGHTS,
];

/** Sidebar groups below the main destinations. */
export const SIDEBAR_GROUPS: readonly NavGroup[] = [
  { label: "People", items: [OWED] },
  { label: "Organize", items: [CATEGORIES, RULES] },
];

/** Everything the More page lists (what the four main tabs don't reach). */
export const MORE_NAV: readonly NavItem[] = [
  INVESTMENTS,
  BUDGETS,
  INSIGHTS,
  OWED,
  CATEGORIES,
  RULES,
];

/** Palette navigation entries: main destinations, people, organize. */
export const PALETTE_NAV: readonly NavItem[] = [
  ...MAIN_NAV,
  OWED,
  CATEGORIES,
  RULES,
];

function onPath(pathname: string, href: string): boolean {
  return href === "/"
    ? pathname === "/"
    : pathname === href || pathname.startsWith(`${href}/`);
}

/** True when `pathname` is on `item` (or one of its extra prefixes). */
export function isNavActive(pathname: string, item: NavItem): boolean {
  return (
    onPath(pathname, item.href) ||
    (item.also?.some((p) => onPath(pathname, p)) ?? false)
  );
}

const TITLES: readonly (readonly [string, string])[] = [
  ["/accounts", "Account"],
  ["/investments", "Holding"],
  ["/securities", "Security"],
];

const LIST_TITLES: Readonly<Record<string, string>> = {
  "/": "Home",
  "/accounts": "Accounts",
  "/transactions": "Transactions",
  "/investments": "Investments",
  "/securities": "Securities",
  "/budgets": "Budgets",
  "/analytics": "Insights",
  "/receivables": "Owed to me",
  "/categories": "Categories",
  "/rules": "Rules",
  "/settings": "Settings",
  "/more": "More",
};

/** Mobile header title for a dashboard path. */
export function pageTitle(pathname: string): string {
  const exact = LIST_TITLES[pathname];
  if (exact) return exact;
  const detail = TITLES.find(([prefix]) => pathname.startsWith(`${prefix}/`));
  return detail ? detail[1] : "Kuberan";
}

export type TabKey = "home" | "activity" | "wealth" | "more";

/** Which bottom tab a path belongs to. */
export function activeTab(pathname: string): TabKey {
  if (pathname === "/") return "home";
  if (onPath(pathname, "/transactions")) return "activity";
  if (
    onPath(pathname, "/accounts") ||
    onPath(pathname, "/investments") ||
    onPath(pathname, "/securities")
  ) {
    return "wealth";
  }
  return "more";
}
