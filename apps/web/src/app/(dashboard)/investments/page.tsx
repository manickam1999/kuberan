"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Plus,
} from "lucide-react";

import { WealthSwitch } from "@/components/layout/wealth-switch";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  usePortfolio,
  useAllInvestments,
  usePrefetchInvestment,
  type InvestmentStatus,
} from "@/hooks/use-investments";
import { assetTypeLabel } from "@/lib/asset-types";
import { signedPercent, weightPct } from "@/lib/price-series";
import { cn } from "@/lib/utils";
import {
  UnrealizedPnl,
  holdingFigures,
  holdingReturnPct,
} from "@/components/investments/holding-figures";
import { CurrencyExposureChart } from "@/components/investments/currency-exposure-chart";
import { AssetAllocationChart } from "@/components/investments/asset-allocation-chart";
import { PortfolioHero } from "@/components/investments/portfolio-hero";
import { ReturnBreakdown } from "@/components/investments/return-breakdown";
import { RecordTradeSheet } from "@/components/investments/record-trade-sheet";
import { SymbolTile } from "@/components/investments/symbol-tile";
import type { Investment } from "@/types/models";

const HOLDINGS_PAGE_SIZE = 20;

type SortColumn =
  | "symbol"
  | "qty"
  | "price"
  | "market_value"
  | "unrealized_gl"
  | "realized_gl"
  | "total_invested"
  | "return_pct";
type SortDirection = "asc" | "desc" | null;

/**
 * Approximate base-currency size of a native amount at the holding's current
 * rate. Used for sorting only, never displayed.
 */
function approxBase(investment: Investment, native: number): number {
  if (investment.fx_missing) return 0;
  const rate = investment.fx_rate ? Number(investment.fx_rate) : 1;
  return native * (Number.isFinite(rate) && rate > 0 ? rate : 1);
}

/** Realised return of a closed position, as a percentage of what was invested. */
function closedReturnPct(investment: Investment) {
  if (investment.total_invested === 0) return 0;
  return (investment.realized_gain_loss / investment.total_invested) * 100;
}

function qtyText(quantity: number) {
  return quantity.toLocaleString(undefined, { maximumFractionDigits: 6 });
}

function toneOf(n: number) {
  return n > 0 ? "text-positive" : n < 0 ? "text-negative" : "";
}

/** Mobile list row of an open holding. */
function HoldingRow({
  investment,
  onPrefetch,
}: {
  investment: Investment;
  onPrefetch: () => void;
}) {
  const f = holdingFigures(investment);
  const pct = holdingReturnPct(investment);
  return (
    <li>
      <Link
        href={`/investments/${investment.id}`}
        onMouseEnter={onPrefetch}
        onFocus={onPrefetch}
        className="hover:bg-accent/40 focus-visible:ring-ring/50 flex items-center gap-3 px-4 py-3 outline-none focus-visible:ring-[3px] focus-visible:ring-inset"
      >
        <SymbolTile
          symbol={investment.security.symbol}
          assetType={investment.security.asset_type}
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {investment.security.name}
          </span>
          <span className="text-muted-foreground block truncate text-xs">
            {qtyText(investment.quantity)} ×{" "}
            <Money
              amount={investment.current_price_native}
              currency={f.currency}
              fxMissing={investment.fx_missing}
            />
          </span>
        </span>
        <span className="flex shrink-0 flex-col items-end gap-0.5 text-sm">
          <Money
            amount={f.nativeValue}
            currency={f.currency}
            baseAmount={f.baseValue}
            fxRate={investment.fx_rate}
            fxStale={investment.fx_stale}
            fxMissing={investment.fx_missing}
            layout="stacked"
            className="items-end font-medium"
          />
          <span className={cn("text-xs tabular-nums", toneOf(pct))}>
            {signedPercent(pct, 0)}
          </span>
        </span>
      </Link>
    </li>
  );
}

/** Mobile list row of a closed position. */
function ClosedRow({
  investment,
  onPrefetch,
}: {
  investment: Investment;
  onPrefetch: () => void;
}) {
  const pct = closedReturnPct(investment);
  return (
    <li>
      <Link
        href={`/investments/${investment.id}`}
        onMouseEnter={onPrefetch}
        onFocus={onPrefetch}
        className="hover:bg-accent/40 focus-visible:ring-ring/50 flex items-center gap-3 px-4 py-3 outline-none focus-visible:ring-[3px] focus-visible:ring-inset"
      >
        <SymbolTile
          symbol={investment.security.symbol}
          assetType={investment.security.asset_type}
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {investment.security.name}
          </span>
          <span className="text-muted-foreground block truncate text-xs">
            Invested{" "}
            <Money amount={investment.total_invested} currency={investment.currency} />
          </span>
        </span>
        <span className="flex shrink-0 flex-col items-end gap-0.5 text-sm">
          <Money
            amount={investment.realized_gain_loss}
            currency={investment.currency}
            baseAmount={investment.realized_gain_loss_base}
            sign="always"
            layout="stacked"
            className={cn("items-end font-medium", toneOf(investment.realized_gain_loss))}
          />
          <span className={cn("text-xs tabular-nums", toneOf(pct))}>
            {signedPercent(pct, 0)}
          </span>
        </span>
      </Link>
    </li>
  );
}

function HoldingsSkeleton() {
  return (
    <>
      <div className="divide-border/60 divide-y md:hidden">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="flex items-center gap-3 px-4 py-3">
            <Skeleton className="size-9 rounded-control" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-3 w-1/3" />
            </div>
            <Skeleton className="h-8 w-20" />
          </div>
        ))}
      </div>
      <div className="hidden space-y-3 p-4 md:block">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    </>
  );
}

function HoldingsSection({ totalValue }: { totalValue: number }) {
  const router = useRouter();
  const prefetchInvestment = usePrefetchInvestment();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<InvestmentStatus>("open");
  const [sortColumn, setSortColumn] = useState<SortColumn | null>(null);
  const [sortDirection, setSortDirection] = useState<SortDirection>(null);
  const { data, isLoading, isError, refetch } = useAllInvestments({
    page,
    page_size: HOLDINGS_PAGE_SIZE,
    status,
  });

  const handleTabChange = (next: "open" | "closed" | "securities") => {
    if (next === "securities") {
      router.push("/securities");
      return;
    }
    if (next === status) return;
    setStatus(next);
    setPage(1);
    setSortColumn(null);
    setSortDirection(null);
  };

  const investments = useMemo(() => data?.data ?? [], [data?.data]);
  const totalPages = data?.total_pages ?? 1;

  const handleSort = (column: SortColumn) => {
    if (sortColumn !== column) {
      setSortColumn(column);
      setSortDirection("asc");
    } else if (sortDirection === "asc") {
      setSortDirection("desc");
    } else {
      setSortColumn(null);
      setSortDirection(null);
    }
  };

  const sortedInvestments = useMemo(() => {
    if (!sortColumn || !sortDirection) return investments;
    const dir = sortDirection === "asc" ? 1 : -1;
    return [...investments].sort((a, b) => {
      switch (sortColumn) {
        case "symbol":
          return dir * a.security.symbol.localeCompare(b.security.symbol);
        case "qty":
          return dir * (a.quantity - b.quantity);
        case "price":
          // The base price keeps holdings in different currencies comparable.
          return dir * (a.current_price - b.current_price);
        case "market_value":
          return (
            dir *
            (Math.round(a.quantity * a.current_price) -
              Math.round(b.quantity * b.current_price))
          );
        case "unrealized_gl":
          return dir * ((holdingFigures(a).basePnl ?? 0) - (holdingFigures(b).basePnl ?? 0));
        case "realized_gl":
          return dir * (a.realized_gain_loss_base - b.realized_gain_loss_base);
        case "total_invested":
          return (
            dir *
            (approxBase(a, a.total_invested) - approxBase(b, b.total_invested))
          );
        case "return_pct":
          return dir * (closedReturnPct(a) - closedReturnPct(b));
      }
    });
  }, [investments, sortColumn, sortDirection]);

  const isClosed = status === "closed";
  const tab: "open" | "closed" | "securities" = status === "closed" ? "closed" : "open";

  const sortableHeader = (column: SortColumn, label: string, right?: boolean) => {
    const isActive = sortColumn === column;
    const Icon =
      isActive && sortDirection === "asc"
        ? ArrowUp
        : isActive && sortDirection === "desc"
          ? ArrowDown
          : ArrowUpDown;
    return (
      <TableHead
        key={column}
        aria-sort={
          isActive
            ? sortDirection === "asc"
              ? "ascending"
              : "descending"
            : undefined
        }
        className={right ? "text-right" : undefined}
      >
        <button
          type="button"
          onClick={() => handleSort(column)}
          className={cn(
            "focus-visible:ring-ring/50 inline-flex items-center gap-1 rounded-sm outline-none select-none focus-visible:ring-[3px]",
            right && "flex-row-reverse"
          )}
        >
          {label}
          <Icon
            className={cn(
              "size-3",
              isActive ? "text-foreground" : "text-muted-foreground/50"
            )}
          />
        </button>
      </TableHead>
    );
  };

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-semibold">Holdings</h2>
        <Segmented
          aria-label="Holdings view"
          value={tab}
          onValueChange={handleTabChange}
          options={[
            { value: "open", label: "Current" },
            { value: "closed", label: "Closed" },
            { value: "securities", label: "Securities" },
          ]}
        />
      </div>

      <Card className="gap-0 overflow-hidden py-0">
        {isLoading ? (
          <HoldingsSkeleton />
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 p-10 text-center">
            <p className="text-muted-foreground text-sm">
              Couldn&apos;t load your holdings.
            </p>
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              Try again
            </Button>
          </div>
        ) : investments.length === 0 ? (
          <div className="flex flex-col items-center gap-3 p-10 text-center">
            <p className="text-muted-foreground text-sm">
              {isClosed
                ? "Positions you fully sell will appear here with their realised gain."
                : "No holdings yet. Open an investment account to add your first."}
            </p>
            {!isClosed && (
              <Button size="sm" asChild>
                <Link href="/accounts">Go to Accounts</Link>
              </Button>
            )}
          </div>
        ) : (
          <>
            {/* Phones: list */}
            <ul className="divide-border/60 divide-y md:hidden">
              {sortedInvestments.map((inv) =>
                isClosed ? (
                  <ClosedRow
                    key={inv.id}
                    investment={inv}
                    onPrefetch={() => prefetchInvestment(inv.id)}
                  />
                ) : (
                  <HoldingRow
                    key={inv.id}
                    investment={inv}
                    onPrefetch={() => prefetchInvestment(inv.id)}
                  />
                )
              )}
            </ul>

            {/* Desktop: sortable table */}
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    {sortableHeader("symbol", "Holding")}
                    <TableHead>Type</TableHead>
                    <TableHead>Account</TableHead>
                    {isClosed ? (
                      <>
                        {sortableHeader("total_invested", "Invested", true)}
                        {sortableHeader("realized_gl", "Realised gain", true)}
                        {sortableHeader("return_pct", "Return", true)}
                      </>
                    ) : (
                      <>
                        {sortableHeader("qty", "Qty", true)}
                        {sortableHeader("price", "Price", true)}
                        {sortableHeader("market_value", "Value", true)}
                        {sortableHeader("unrealized_gl", "Gain", true)}
                        <TableHead className="text-right">Weight</TableHead>
                      </>
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedInvestments.map((inv) => {
                    const f = holdingFigures(inv);
                    const weight =
                      f.baseValue === null ? null : weightPct(f.baseValue, totalValue);
                    return (
                      <TableRow
                        key={inv.id}
                        className="cursor-pointer"
                        onClick={() => router.push(`/investments/${inv.id}`)}
                        onMouseEnter={() => prefetchInvestment(inv.id)}
                      >
                        <TableCell>
                          <Link
                            href={`/investments/${inv.id}`}
                            onClick={(e) => e.stopPropagation()}
                            className="focus-visible:ring-ring/50 flex items-center gap-3 rounded-sm outline-none focus-visible:ring-[3px]"
                          >
                            <SymbolTile
                              symbol={inv.security.symbol}
                              assetType={inv.security.asset_type}
                            />
                            <span className="min-w-0">
                              <span className="block font-mono text-xs font-semibold">
                                {inv.security.symbol}
                              </span>
                              <span className="block max-w-56 truncate text-sm font-medium">
                                {inv.security.name}
                              </span>
                            </span>
                          </Link>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {assetTypeLabel(inv.security.asset_type)}
                        </TableCell>
                        <TableCell>
                          <Link
                            href={`/accounts/${inv.account_id}`}
                            className="text-primary hover:underline"
                            onClick={(e) => e.stopPropagation()}
                          >
                            {inv.account?.name ?? `Account #${inv.account_id}`}
                          </Link>
                        </TableCell>
                        {isClosed ? (
                          <>
                            <TableCell className="text-right">
                              <Money
                                amount={inv.total_invested}
                                currency={inv.currency}
                              />
                            </TableCell>
                            <TableCell className="text-right font-medium">
                              <Money
                                amount={inv.realized_gain_loss}
                                currency={inv.currency}
                                baseAmount={inv.realized_gain_loss_base}
                                sign="always"
                                className={cn("justify-end", toneOf(inv.realized_gain_loss))}
                              />
                            </TableCell>
                            <TableCell
                              className={cn(
                                "text-right font-medium tabular-nums",
                                toneOf(inv.realized_gain_loss)
                              )}
                            >
                              {signedPercent(closedReturnPct(inv))}
                            </TableCell>
                          </>
                        ) : (
                          <>
                            <TableCell className="text-right tabular-nums">
                              {qtyText(inv.quantity)}
                            </TableCell>
                            <TableCell className="text-right">
                              <Money
                                amount={inv.current_price_native}
                                currency={f.currency}
                                fxRate={inv.fx_rate}
                                fxStale={inv.fx_stale}
                                fxMissing={inv.fx_missing}
                              />
                            </TableCell>
                            <TableCell className="text-right font-medium">
                              <Money
                                amount={f.nativeValue}
                                currency={f.currency}
                                baseAmount={f.baseValue}
                                fxRate={inv.fx_rate}
                                fxStale={inv.fx_stale}
                                fxMissing={inv.fx_missing}
                                className="justify-end"
                              />
                            </TableCell>
                            <TableCell className="text-right font-medium">
                              <UnrealizedPnl investment={inv} className="justify-end" />
                              <span
                                className={cn(
                                  "text-muted-foreground block text-xs font-normal tabular-nums"
                                )}
                              >
                                {signedPercent(holdingReturnPct(inv))}
                              </span>
                            </TableCell>
                            <TableCell className="text-right">
                              {weight === null ? (
                                <span className="text-muted-foreground">—</span>
                              ) : (
                                <span className="inline-flex items-center justify-end gap-2">
                                  <span
                                    aria-hidden
                                    className="bg-muted relative h-1.5 w-14 overflow-hidden rounded-full"
                                  >
                                    <i
                                      className="bg-primary absolute inset-y-0 left-0 rounded-full"
                                      style={{ width: `${Math.min(weight, 100)}%` }}
                                    />
                                  </span>
                                  <span className="text-muted-foreground w-9 text-xs tabular-nums">
                                    {weight}%
                                  </span>
                                </span>
                              )}
                            </TableCell>
                          </>
                        )}
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between border-t px-4 py-3">
                <span className="text-muted-foreground text-sm">
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
          </>
        )}
      </Card>
    </section>
  );
}

function InvestmentsSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-9 w-full md:hidden" />
      <div className="hidden items-center justify-between md:flex">
        <div className="space-y-2">
          <Skeleton className="h-9 w-48" />
          <Skeleton className="h-4 w-64" />
        </div>
        <Skeleton className="h-9 w-32" />
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        <Skeleton className="h-[480px] rounded-card lg:col-span-2" />
        <Skeleton className="h-[480px] rounded-card" />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-60 rounded-card" />
        <Skeleton className="h-60 rounded-card" />
      </div>
      <Skeleton className="h-64 rounded-card" />
    </div>
  );
}

export default function InvestmentsPage() {
  const { data: portfolio, isLoading, isError, refetch } = usePortfolio();
  const { data: allHoldings } = useAllInvestments({
    page: 1,
    page_size: 1,
    status: "all",
  });
  const [tradeOpen, setTradeOpen] = useState(false);

  if (isLoading) return <InvestmentsSkeleton />;

  if (isError || !portfolio) {
    return (
      <div className="space-y-5">
        <WealthSwitch current="/investments" />
        <h1 className="font-display hidden text-3xl leading-tight md:block">
          Investments
        </h1>
        <div className="flex flex-col items-center gap-3 rounded-card border border-dashed p-12 text-center">
          <p className="font-medium">Couldn&apos;t load your portfolio</p>
          <p className="text-muted-foreground text-sm">
            Something went wrong while loading it.
          </p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Try again
          </Button>
        </div>
      </div>
    );
  }

  const holdingsTotal = allHoldings?.total_items;
  const classCount = Object.values(portfolio.holdings_by_type).filter(
    (h) => h.count > 0
  ).length;
  const empty = holdingsTotal === 0;

  return (
    <div className="space-y-5">
      <WealthSwitch current="/investments" />

      <div className="flex items-center justify-between gap-3">
        <div className="hidden md:block">
          <h1 className="font-display text-3xl leading-tight">Investments</h1>
          {holdingsTotal !== undefined && !empty && (
            <p className="text-muted-foreground text-sm">
              {holdingsTotal} holding{holdingsTotal !== 1 ? "s" : ""}
              {classCount > 0 &&
                ` across ${classCount} asset class${classCount !== 1 ? "es" : ""}`}
            </p>
          )}
        </div>
        {!empty && (
          <Button
            size="sm"
            className="ml-auto hidden md:inline-flex"
            onClick={() => setTradeOpen(true)}
          >
            <Plus className="size-4" />
            Record trade
          </Button>
        )}
      </div>

      {empty ? (
        <div className="flex flex-col items-center gap-3 rounded-card border border-dashed p-12 text-center">
          <p className="font-medium">No investments yet</p>
          <p className="text-muted-foreground max-w-sm text-sm">
            Add an investment account, then record your first buy to see your
            portfolio here.
          </p>
          <Button size="sm" asChild>
            <Link href="/accounts">Go to Accounts</Link>
          </Button>
        </div>
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-3">
            <div className="min-w-0 lg:col-span-2">
              <PortfolioHero portfolio={portfolio} />
            </div>
            <div className="min-w-0">
              <AssetAllocationChart
                holdingsByType={portfolio.holdings_by_type}
                totalValue={portfolio.total_value}
                currency={portfolio.currency}
              />
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <ReturnBreakdown portfolio={portfolio} />
            <CurrencyExposureChart />
          </div>

          <HoldingsSection totalValue={portfolio.total_value} />
        </>
      )}

      <RecordTradeSheet open={tradeOpen} onOpenChange={setTradeOpen} />
    </div>
  );
}
