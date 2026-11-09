"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";

import {
  usePrefetchSecurity,
  useAllSecurities,
  useSecurityPriceHistory,
} from "@/hooks/use-securities";
import { useAllInvestments } from "@/hooks/use-investments";
import { assetTypeLabel } from "@/lib/asset-types";
import {
  isoDay,
  lastChangePct,
  signedPercent,
  toPricePoints,
} from "@/lib/price-series";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleChip } from "@/components/ui/toggle-chip";
import { PriceSparkline } from "@/components/investments/price-sparkline";
import { SymbolTile } from "@/components/investments/symbol-tile";
import type { Security } from "@/types/models";

/** Rows shown before "Show more"; each row loads its own recent prices. */
const ROWS_PER_STEP = 25;
/** How far back a row looks for its sparkline and latest price. */
const QUOTE_DAYS = 45;

type Filter = "all" | "held" | string;

/** Price, sparkline and last move of one row, from a small price-history request. */
function RowQuote({ security }: { security: Security }) {
  const { from, to } = useMemo(() => {
    const now = new Date();
    const from = new Date(now);
    from.setUTCDate(from.getUTCDate() - QUOTE_DAYS);
    const to = new Date(now);
    to.setUTCDate(to.getUTCDate() + 1);
    return { from: isoDay(from), to: isoDay(to) };
  }, []);
  const { data, isLoading } = useSecurityPriceHistory(security.id, from, to, {
    page: 1,
    page_size: 100,
  });
  const points = useMemo(() => toPricePoints(data?.data ?? []), [data?.data]);

  if (isLoading) {
    return <Skeleton className="h-9 w-24" />;
  }
  if (points.length === 0) {
    return <span className="text-muted-foreground text-xs">No price yet</span>;
  }
  const last = points[points.length - 1];
  const change = lastChangePct(points);
  return (
    <>
      <PriceSparkline
        values={points.map((p) => p.cents)}
        className="hidden md:block"
      />
      <span className="flex min-w-20 shrink-0 flex-col items-end gap-0.5 text-sm">
        <Money amount={last.cents} currency={security.currency} className="font-medium" />
        {change !== null && (
          <span
            className={cn(
              "text-xs tabular-nums",
              change > 0 ? "text-positive" : change < 0 ? "text-negative" : "text-muted-foreground"
            )}
            title="Change since the previous price"
          >
            {signedPercent(change)}
          </span>
        )}
      </span>
    </>
  );
}

function SecurityRow({
  security,
  held,
  onPrefetch,
}: {
  security: Security;
  held: boolean;
  onPrefetch: () => void;
}) {
  const details = [
    assetTypeLabel(security.asset_type),
    security.exchange,
    security.currency,
  ].filter(Boolean);
  return (
    <li>
      <Link
        href={`/securities/${security.id}`}
        onMouseEnter={onPrefetch}
        onFocus={onPrefetch}
        className="hover:bg-accent/40 focus-visible:ring-ring/50 flex items-center gap-3 px-4 py-3 outline-none focus-visible:ring-[3px] focus-visible:ring-inset md:px-5"
      >
        <SymbolTile symbol={security.symbol} assetType={security.asset_type} />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="truncate text-sm font-medium">{security.name}</span>
            {held && <Badge tone="accent">Held</Badge>}
          </span>
          <span className="text-muted-foreground block truncate text-xs">
            <span className="font-mono">{security.symbol}</span> ·{" "}
            {details.join(" · ")}
          </span>
        </span>
        <RowQuote security={security} />
        <ChevronRight className="text-muted-foreground size-4 shrink-0" aria-hidden />
      </Link>
    </li>
  );
}

function ListSkeleton() {
  return (
    <Card className="divide-border/60 gap-0 divide-y py-0" aria-busy="true" aria-label="Loading">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-4 py-3 md:px-5">
          <Skeleton className="size-9 rounded-control" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-3 w-1/3" />
          </div>
          <Skeleton className="hidden h-7 w-28 md:block" />
          <Skeleton className="h-9 w-20" />
        </div>
      ))}
    </Card>
  );
}

export default function SecuritiesPage() {
  const prefetchSecurity = usePrefetchSecurity();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [visible, setVisible] = useState(ROWS_PER_STEP);

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search.trim());
      setVisible(ROWS_PER_STEP);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data, isLoading, isError, refetch } = useAllSecurities(debouncedSearch);
  const { data: held } = useAllInvestments({
    page: 1,
    page_size: 100,
    status: "open",
  });
  const heldIds = useMemo(
    () => new Set((held?.data ?? []).map((h) => h.security_id)),
    [held?.data]
  );

  const securities = useMemo(() => data ?? [], [data]);
  const types = useMemo(
    () => [...new Set(securities.map((s) => s.asset_type.toLowerCase()))].sort(),
    [securities]
  );
  const filtered = useMemo(
    () =>
      securities.filter((s) =>
        filter === "all"
          ? true
          : filter === "held"
            ? heldIds.has(s.id)
            : s.asset_type.toLowerCase() === filter
      ),
    [securities, filter, heldIds]
  );
  const shown = filtered.slice(0, visible);

  const choose = (next: Filter) => {
    setFilter(next);
    setVisible(ROWS_PER_STEP);
  };

  return (
    <div className="space-y-5">
      <div className="space-y-1">
        <Link
          href="/investments"
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 -ml-1 inline-flex min-h-9 items-center gap-0.5 rounded-md px-1 text-sm outline-none focus-visible:ring-[3px]"
        >
          <ChevronLeft className="size-4" />
          Investments
        </Link>
        <h1 className="font-display hidden text-3xl leading-tight md:block">
          Securities
        </h1>
        <p className="text-muted-foreground text-sm">
          {securities.length} tracked · prices update daily
        </p>
      </div>

      <div className="flex flex-col gap-3 md:flex-row md:items-center">
        <div className="relative w-full md:max-w-xs">
          <Search
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
            aria-hidden
          />
          <Input
            type="search"
            aria-label="Search securities"
            placeholder="Symbol or name"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-10 pl-9 md:h-9"
          />
        </div>
        <div
          role="group"
          aria-label="Filter securities"
          className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 md:mx-0 md:px-0 md:pb-0"
        >
          <ToggleChip pressed={filter === "all"} onPressedChange={() => choose("all")}>
            All
          </ToggleChip>
          <ToggleChip pressed={filter === "held"} onPressedChange={() => choose("held")}>
            Held
          </ToggleChip>
          {types.map((t) => (
            <ToggleChip key={t} pressed={filter === t} onPressedChange={() => choose(t)}>
              {assetTypeLabel(t)}
            </ToggleChip>
          ))}
        </div>
      </div>

      {isLoading ? (
        <ListSkeleton />
      ) : isError ? (
        <div className="flex flex-col items-center gap-3 rounded-card border border-dashed p-12 text-center">
          <p className="text-muted-foreground text-sm">
            Couldn&apos;t load the securities.
          </p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Try again
          </Button>
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-card border border-dashed p-12 text-center">
          <p className="text-muted-foreground text-sm">
            {securities.length === 0 && !debouncedSearch
              ? "No securities are tracked yet. They appear here once a holding uses them."
              : filter === "held"
                ? "You don't hold any security that matches."
                : "No securities match."}
          </p>
          {securities.length === 0 && !debouncedSearch ? (
            <Button size="sm" asChild>
              <Link href="/investments">Go to Investments</Link>
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setSearch("");
                setDebouncedSearch("");
                choose("all");
              }}
            >
              Clear search and filters
            </Button>
          )}
        </div>
      ) : (
        <>
          <Card className="gap-0 overflow-hidden py-0">
            <ul className="divide-border/60 divide-y">
              {shown.map((security) => (
                <SecurityRow
                  key={security.id}
                  security={security}
                  held={heldIds.has(security.id)}
                  onPrefetch={() => prefetchSecurity(security.id)}
                />
              ))}
            </ul>
          </Card>
          {filtered.length > shown.length && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setVisible((v) => v + ROWS_PER_STEP)}
              >
                Show more ({filtered.length - shown.length})
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
