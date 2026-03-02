"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";

import { useSecurities } from "@/hooks/use-securities";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { AssetType, Security } from "@/types/models";

const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  bond: "Bond",
  crypto: "Crypto",
  reit: "REIT",
  commodity: "Commodity",
};

const PAGE_SIZE = 20;

function SecuritiesTableSkeleton() {
  return (
    <>
      {/* Mobile: Card skeletons */}
      <div className="md:hidden grid gap-3">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-32 w-full rounded-lg" />
        ))}
      </div>

      {/* Desktop: Table skeleton */}
      <div className="hidden md:block space-y-3">
        <Skeleton className="h-10 w-full" />
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    </>
  );
}

function SecurityCard({ security }: { security: Security }) {
  const assetType = security.asset_type.toLowerCase() as AssetType;

  return (
    <Link href={`/securities/${security.id}`}>
      <Card className="transition-colors hover:bg-accent/50 cursor-pointer">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0 flex-1">
              <CardTitle className="text-base font-mono truncate">
                {security.symbol}
              </CardTitle>
              <p className="text-sm text-muted-foreground truncate">
                {security.name}
              </p>
            </div>
            <Badge variant="secondary" className="shrink-0">
              {ASSET_TYPE_LABELS[assetType] ?? security.asset_type}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span>{security.currency}</span>
            {security.exchange && (
              <>
                <span>·</span>
                <span>{security.exchange}</span>
              </>
            )}
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

function SecurityRow({
  security,
  onClick,
}: {
  security: Security;
  onClick: () => void;
}) {
  return (
    <TableRow className="cursor-pointer" onClick={onClick}>
      <TableCell className="font-mono font-semibold">
        {security.symbol}
      </TableCell>
      <TableCell>{security.name}</TableCell>
      <TableCell>
        <Badge variant="outline">
          {ASSET_TYPE_LABELS[security.asset_type.toLowerCase() as AssetType] ??
            security.asset_type}
        </Badge>
      </TableCell>
      <TableCell>{security.currency}</TableCell>
      <TableCell className="text-muted-foreground">
        {security.exchange || "-"}
      </TableCell>
    </TableRow>
  );
}

export default function SecuritiesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(1);

  // Debounce search input (300ms)
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
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Securities</h1>
      </div>

      {/* Search */}
      <div className="relative max-w-sm">
        <Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
        <Input
          placeholder="Search by symbol or name..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>

      {/* Content */}
      {isLoading ? (
        <SecuritiesTableSkeleton />
      ) : securities.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">No securities found</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {debouncedSearch
              ? "No securities match your search. Try a different term."
              : "No securities are available yet."}
          </p>
        </div>
      ) : (
        <>
          {/* Mobile: Card grid */}
          <div className="md:hidden grid gap-3">
            {securities.map((security) => (
              <SecurityCard key={security.id} security={security} />
            ))}
          </div>

          {/* Desktop: Table */}
          <div className="hidden md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Symbol</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Currency</TableHead>
                  <TableHead>Exchange</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {securities.map((security) => (
                  <SecurityRow
                    key={security.id}
                    security={security}
                    onClick={() => router.push(`/securities/${security.id}`)}
                  />
                ))}
              </TableBody>
            </Table>
          </div>

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
                  <ChevronLeft className="h-4 w-4" />
                  <span className="ml-1 hidden sm:inline">Previous</span>
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  <span className="mr-1 hidden sm:inline">Next</span>
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
