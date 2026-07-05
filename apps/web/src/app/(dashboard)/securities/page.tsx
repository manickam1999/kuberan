"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";

import { useSecurities } from "@/hooks/use-securities";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import type { AssetType, Security } from "@/types/models";

const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  bond: "Bond",
  crypto: "Crypto",
  reit: "REIT",
  commodity: "Commodity",
};

const ASSET_TYPE_COLOR: Record<AssetType, string> = {
  stock: "var(--chart-1)",
  etf: "var(--chart-3)",
  bond: "var(--chart-4)",
  crypto: "var(--chart-2)",
  reit: "var(--chart-5)",
  commodity: "var(--chart-6)",
};

const PAGE_SIZE = 20;

function SecurityRow({
  security,
  onClick,
}: {
  security: Security;
  onClick: () => void;
}) {
  const assetType = security.asset_type.toLowerCase() as AssetType;
  const color = ASSET_TYPE_COLOR[assetType] ?? "var(--muted-foreground)";

  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40"
    >
      <span
        className="flex size-9 shrink-0 items-center justify-center rounded-lg font-mono text-xs font-semibold"
        style={{ backgroundColor: `color-mix(in oklch, ${color} 18%, transparent)`, color }}
      >
        {security.symbol.slice(0, 4)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm font-semibold">
          {security.symbol}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {security.name}
        </p>
      </div>
      <div className="hidden items-center gap-2 text-xs text-muted-foreground sm:flex">
        <span>{security.currency}</span>
        {security.exchange && (
          <>
            <span>·</span>
            <span>{security.exchange}</span>
          </>
        )}
      </div>
      <Badge variant="outline" className="shrink-0">
        {ASSET_TYPE_LABELS[assetType] ?? security.asset_type}
      </Badge>
    </button>
  );
}

export default function SecuritiesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data, isLoading } = useSecurities({
    search: debouncedSearch || undefined,
    page,
    page_size: PAGE_SIZE,
  });

  const securities = data?.data ?? [];
  const totalPages = data?.total_pages ?? 1;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Securities</h1>
          <p className="text-sm text-muted-foreground">
            {data?.total_items ?? securities.length} instruments available
          </p>
        </div>
        <div className="relative w-full max-w-xs">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search symbol or name…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-9 pl-9"
          />
        </div>
      </div>

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : securities.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No securities found</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {debouncedSearch
              ? "No securities match your search. Try a different term."
              : "No securities are available yet."}
          </p>
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          <div className="divide-y divide-border/50">
            {securities.map((security) => (
              <SecurityRow
                key={security.id}
                security={security}
                onClick={() => router.push(`/securities/${security.id}`)}
              />
            ))}
          </div>
        </Card>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              <ChevronLeft className="size-4" />
              <span className="ml-1 hidden sm:inline">Previous</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              <span className="mr-1 hidden sm:inline">Next</span>
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
