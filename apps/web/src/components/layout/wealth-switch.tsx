import Link from "next/link";
import { cn } from "@/lib/utils";

const OPTIONS = [
  { href: "/accounts", label: "Accounts" },
  { href: "/investments", label: "Portfolio" },
] as const;

/**
 * Accounts / Portfolio switch for the phone "Wealth" tab: a pair of links
 * styled like the segmented control. A stand-in until the Phase 4 Wealth
 * screen; hidden from `md` up, where the sidebar lists both pages.
 */
export function WealthSwitch({ current }: { current: "/accounts" | "/investments" }) {
  return (
    <nav
      aria-label="Wealth"
      className="bg-background grid grid-cols-2 gap-0.5 rounded-control border p-0.5 md:hidden"
    >
      {OPTIONS.map((o) => (
        <Link
          key={o.href}
          href={o.href}
          aria-current={o.href === current ? "page" : undefined}
          className={cn(
            "focus-visible:ring-ring/50 inline-flex min-h-9 items-center justify-center rounded-[7px] text-xs font-medium transition-colors outline-none focus-visible:ring-[3px]",
            o.href === current
              ? "bg-muted text-foreground"
              : "text-muted-foreground hover:text-foreground"
          )}
        >
          {o.label}
        </Link>
      ))}
    </nav>
  );
}
