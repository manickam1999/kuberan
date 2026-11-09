"use client";

import { useRouter } from "next/navigation";
import { Segmented } from "@/components/ui/segmented";
import { cn } from "@/lib/utils";

type WealthPath = "/accounts" | "/investments";

const OPTIONS = [
  { value: "/accounts", label: "Accounts" },
  { value: "/investments", label: "Portfolio" },
] as const;

/**
 * Accounts / Portfolio segmented switch for the phone "Wealth" tab (prototype
 * `seg` full-width). Choosing a segment navigates to that page. Hidden from
 * `md` up, where the sidebar lists both pages.
 */
export function WealthSwitch({
  current,
  className,
}: {
  current: WealthPath;
  className?: string;
}) {
  const router = useRouter();
  return (
    <Segmented<WealthPath>
      aria-label="Wealth"
      full
      value={current}
      options={OPTIONS}
      onValueChange={(href) => {
        if (href !== current) router.push(href);
      }}
      className={cn("md:hidden", className)}
    />
  );
}
