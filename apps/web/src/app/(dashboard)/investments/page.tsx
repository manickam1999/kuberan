"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  TrendingUp,
  TrendingDown,
  Wallet,
  Coins,
  Landmark,
  ChevronLeft,
  ChevronRight,
  List,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
} from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatCard } from "@/components/ui/stat-card";
import {
  usePortfolio,
  useAllInvestments,
  usePrefetchInvestment,
  type InvestmentStatus,
} from "@/hooks/use-investments";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { formatMoney, formatPercentage } from "@/lib/format";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import {
  UnrealizedPnl,
  formatSigned,
  holdingFigures,
} from "@/components/investments/holding-figures";
import { CurrencyExposureChart } from "@/components/investments/currency-exposure-chart";
import type { AssetType, Investment } from "@/types/models";
import { AssetAllocationChart } from "@/components/investments/asset-allocation-chart";
import { InvestmentValueChart } from "@/components/investments/investment-value-chart";

function InvestmentsSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-48" />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
      <Skeleton className="h-[350px] w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}

const HOLDINGS_PAGE_SIZE = 20;

type SortColumn =
  | "symbol"
  | "name"
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

function returnPct(investment: Investment) {
  if (investment.total_invested === 0) return 0;
  return (investment.realized_gain_loss / investment.total_invested) * 100;
}

function HoldingCard({
  investment,
  onClick,
  onPrefetch,
}: {
  investment: Investment;
  onClick: () => void;
  onPrefetch: () => void;
}) {
  const f = holdingFigures(investment);
  const isRealizedPositive = investment.realized_gain_loss >= 0;

  return (
    <Card
      className="cursor-pointer transition-colors hover:bg-accent/50"
      onClick={onClick}
      onMouseEnter={onPrefetch}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2 overflow-hidden">
          <div className="min-w-0 flex-shrink overflow-hidden max-w-[50%] sm:max-w-[60%]">
            <CardTitle className="text-base font-mono font-semibold truncate">
              <Link
                href={`/investments/${investment.id}`}
                onClick={(e) => e.stopPropagation()}
              >
                {investment.security.symbol}
              </Link>
            </CardTitle>
            <p className="text-sm text-muted-foreground truncate">
              {investment.security.name}
            </p>
          </div>
          <div className="text-right flex-shrink-0 min-w-0 overflow-hidden">
            <p className="text-sm sm:text-base font-semibold font-mono">
              <Money
                amount={f.nativeValue}
                currency={f.currency}
                baseAmount={f.baseValue}
                fxRate={investment.fx_rate}
                fxStale={investment.fx_stale}
                fxMissing={investment.fx_missing}
                layout="stacked"
                className="items-end"
              />
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 overflow-hidden">
        <div className="grid grid-cols-2 gap-2 text-sm">
          <div className="min-w-0">
            <p className="text-muted-foreground text-xs">Quantity</p>
            <p className="font-medium font-mono tabular-nums truncate">
              {investment.quantity.toFixed(Number.isInteger(investment.quantity) ? 0 : 6)}
            </p>
          </div>
          <div className="text-right min-w-0">
            <p className="text-muted-foreground text-xs">Price</p>
            <p className="font-medium font-mono">
              <Money
                amount={investment.current_price_native}
                currency={f.currency}
                fxRate={investment.fx_rate}
                fxStale={investment.fx_stale}
                fxMissing={investment.fx_missing}
              />
            </p>
          </div>
        </div>
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Unrealized G/L</span>
          <UnrealizedPnl
            investment={investment}
            className="font-medium font-mono"
          />
        </div>
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Realized G/L</span>
          <span
            className={`font-medium font-mono ${
              isRealizedPositive ? "text-positive" : "text-negative"
            }`}
          >
            {isRealizedPositive ? "+" : ""}
            <Money
              amount={investment.realized_gain_loss}
              currency={investment.currency}
              baseAmount={investment.realized_gain_loss_base}
              fxMissing={investment.fx_missing}
            />
          </span>
        </div>
        <div className="pt-1 min-w-0">
          <Link
            href={`/accounts/${investment.account_id}`}
            className="text-xs text-primary hover:underline truncate block"
            onClick={(e) => e.stopPropagation()}
          >
            {investment.account?.name ?? `Account #${investment.account_id}`}
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}

function ClosedHoldingCard({
  investment,
  onClick,
  onPrefetch,
}: {
  investment: Investment;
  onClick: () => void;
  onPrefetch: () => void;
}) {
  const { hideBalances } = useAuth();
  const isRealizedPositive = investment.realized_gain_loss >= 0;
  const pct = returnPct(investment);

  return (
    <Card
      className="cursor-pointer transition-colors hover:bg-accent/50"
      onClick={onClick}
      onMouseEnter={onPrefetch}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2 overflow-hidden">
          <div className="min-w-0 flex-shrink overflow-hidden max-w-[55%] sm:max-w-[65%]">
            <CardTitle className="text-base font-mono font-semibold truncate">
              <Link
                href={`/investments/${investment.id}`}
                onClick={(e) => e.stopPropagation()}
              >
                {investment.security.symbol}
              </Link>
            </CardTitle>
            <p className="text-sm text-muted-foreground truncate">
              {investment.security.name}
            </p>
          </div>
          <div className="text-right flex-shrink-0 min-w-0 overflow-hidden">
            <p
              className={`text-sm sm:text-base font-semibold font-mono ${
                isRealizedPositive ? "text-positive" : "text-negative"
              }`}
            >
              {isRealizedPositive ? "+" : ""}
              <Money
                amount={investment.realized_gain_loss}
                currency={investment.currency}
                baseAmount={investment.realized_gain_loss_base}
                layout="stacked"
                className="items-end"
              />
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 overflow-hidden">
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Invested</span>
          <span className="font-medium font-mono tabular-nums truncate">
            {formatMoney(
              investment.total_invested,
              investment.currency,
              hideBalances
            )}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Return</span>
          <span
            className={`font-medium font-mono tabular-nums truncate ${
              isRealizedPositive ? "text-positive" : "text-negative"
            }`}
          >
            {isRealizedPositive ? "+" : ""}
            {formatPercentage(Math.abs(pct))}
          </span>
        </div>
        <div className="pt-1 min-w-0">
          <Link
            href={`/accounts/${investment.account_id}`}
            className="text-xs text-primary hover:underline truncate block"
            onClick={(e) => e.stopPropagation()}
          >
            {investment.account?.name ?? `Account #${investment.account_id}`}
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}

function AllHoldingsTable() {
  const router = useRouter();
  const { hideBalances } = useAuth();
  const prefetchInvestment = usePrefetchInvestment();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<InvestmentStatus>("open");
  const [sortColumn, setSortColumn] = useState<SortColumn | null>(null);
  const [sortDirection, setSortDirection] = useState<SortDirection>(null);
  const { data, isLoading } = useAllInvestments({
    page,
    page_size: HOLDINGS_PAGE_SIZE,
    status,
  });

  const handleTabChange = (next: string) => {
    if (next !== "open" && next !== "closed") return;
    if (next === status) return;
    setStatus(next);
    setPage(1);
    setSortColumn(null);
    setSortDirection(null);
  };

  const investments = useMemo(() => data?.data ?? [], [data?.data]);
  const totalPages = data?.total_pages ?? 1;
  const totalItems = data?.total_items ?? 0;

  const handleSort = (column: SortColumn) => {
    if (sortColumn !== column) {
      setSortColumn(column);
      setSortDirection("asc");
    } else if (sortDirection === "asc") {
      setSortDirection("desc");
    } else if (sortDirection === "desc") {
      setSortColumn(null);
      setSortDirection(null);
    } else {
      setSortDirection("asc");
    }
  };

  const sortedInvestments = useMemo(() => {
    if (!sortColumn || !sortDirection) return investments;
    return [...investments].sort((a, b) => {
      let aVal: number | string;
      let bVal: number | string;
      switch (sortColumn) {
        case "symbol":
          aVal = a.security.symbol;
          bVal = b.security.symbol;
          break;
        case "name":
          aVal = a.security.name;
          bVal = b.security.name;
          break;
        case "qty":
          aVal = a.quantity;
          bVal = b.quantity;
          break;
        case "price":
          // Base price keeps holdings in different currencies comparable.
          aVal = a.current_price;
          bVal = b.current_price;
          break;
        case "market_value":
          aVal = Math.round(a.quantity * a.current_price);
          bVal = Math.round(b.quantity * b.current_price);
          break;
        case "unrealized_gl":
          aVal = holdingFigures(a).basePnl ?? 0;
          bVal = holdingFigures(b).basePnl ?? 0;
          break;
        case "realized_gl":
          aVal = a.realized_gain_loss_base;
          bVal = b.realized_gain_loss_base;
          break;
        case "total_invested":
          aVal = approxBase(a, a.total_invested);
          bVal = approxBase(b, b.total_invested);
          break;
        case "return_pct":
          aVal = returnPct(a);
          bVal = returnPct(b);
          break;
        default:
          return 0;
      }
      if (typeof aVal === "string" && typeof bVal === "string") {
        return sortDirection === "asc"
          ? aVal.localeCompare(bVal)
          : bVal.localeCompare(aVal);
      }
      return sortDirection === "asc"
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number);
    });
  }, [investments, sortColumn, sortDirection]);

  const SortableHeader = ({
    column,
    label,
    align = "",
  }: {
    column: SortColumn;
    label: string;
    align?: string;
  }) => {
    const isActive = sortColumn === column;
    const Icon =
      isActive && sortDirection === "asc"
        ? ArrowUp
        : isActive && sortDirection === "desc"
          ? ArrowDown
          : ArrowUpDown;
    return (
      <TableHead
        className={`cursor-pointer select-none ${align}`}
        onClick={() => handleSort(column)}
      >
        <div
          className={`flex items-center gap-1 ${align === "text-right" ? "justify-end" : ""}`}
        >
          {label}
          <Icon
            className={`h-3 w-3 ${isActive ? "text-foreground" : "text-muted-foreground/50"}`}
          />
        </div>
      </TableHead>
    );
  };

  const isClosed = status === "closed";
  const itemNoun = isClosed ? "closed position" : "holding";

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2">
            <List className="h-4 w-4 text-muted-foreground" />
            <CardTitle>All Holdings</CardTitle>
          </div>
          <Tabs value={status} onValueChange={handleTabChange}>
            <TabsList>
              <TabsTrigger value="open">Current</TabsTrigger>
              <TabsTrigger value="closed">Closed</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        <CardDescription>
          {totalItems} {itemNoun}
          {totalItems !== 1 ? "s" : ""} across all investment accounts
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <>
            {/* Mobile: Card skeletons */}
            <div className="md:hidden grid gap-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-40 w-full rounded-lg" />
              ))}
            </div>
            {/* Desktop: Table skeletons */}
            <div className="hidden md:block space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          </>
        ) : investments.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
            <h3 className="text-lg font-semibold">
              {isClosed ? "No closed positions yet" : "No holdings yet"}
            </h3>
            <p className="mt-1 text-sm text-muted-foreground">
              {isClosed
                ? "Positions you fully sell will appear here with their realized P/L."
                : "Add investments to your accounts to see them here."}
            </p>
          </div>
        ) : (
          <>
            {/* Mobile: Cards */}
            <div className="md:hidden grid gap-3">
              {sortedInvestments.map((inv) =>
                isClosed ? (
                  <ClosedHoldingCard
                    key={inv.id}
                    investment={inv}
                    onClick={() => router.push(`/investments/${inv.id}`)}
                    onPrefetch={() => prefetchInvestment(inv.id)}
                  />
                ) : (
                  <HoldingCard
                    key={inv.id}
                    investment={inv}
                    onClick={() => router.push(`/investments/${inv.id}`)}
                    onPrefetch={() => prefetchInvestment(inv.id)}
                  />
                )
              )}
            </div>

            {/* Desktop: Sortable Table */}
            <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <SortableHeader column="symbol" label="Symbol" />
                    <SortableHeader column="name" label="Name" />
                    <TableHead>Account</TableHead>
                    {isClosed ? (
                      <>
                        <SortableHeader
                          column="total_invested"
                          label="Invested"
                          align="text-right"
                        />
                        <SortableHeader
                          column="realized_gl"
                          label="Realized G/L"
                          align="text-right"
                        />
                        <SortableHeader
                          column="return_pct"
                          label="Return"
                          align="text-right"
                        />
                      </>
                    ) : (
                      <>
                        <SortableHeader
                          column="qty"
                          label="Qty"
                          align="text-right"
                        />
                        <SortableHeader
                          column="price"
                          label="Price"
                          align="text-right"
                        />
                        <SortableHeader
                          column="market_value"
                          label="Market Value"
                          align="text-right"
                        />
                        <SortableHeader
                          column="unrealized_gl"
                          label="Unrealized G/L"
                          align="text-right"
                        />
                        <SortableHeader
                          column="realized_gl"
                          label="Realized G/L"
                          align="text-right"
                        />
                      </>
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedInvestments.map((inv) => {
                    const realizedPositive = inv.realized_gain_loss >= 0;
                    if (isClosed) {
                      const pct = returnPct(inv);
                      return (
                        <TableRow
                          key={inv.id}
                          className="cursor-pointer"
                          onClick={() => router.push(`/investments/${inv.id}`)}
                          onMouseEnter={() => prefetchInvestment(inv.id)}
                        >
                          <TableCell className="font-mono font-semibold">
                            <Link
                              href={`/investments/${inv.id}`}
                              onClick={(e) => e.stopPropagation()}
                            >
                              {inv.security.symbol}
                            </Link>
                          </TableCell>
                          <TableCell>{inv.security.name}</TableCell>
                          <TableCell>
                            <Link
                              href={`/accounts/${inv.account_id}`}
                              className="text-primary hover:underline"
                              onClick={(e) => e.stopPropagation()}
                            >
                              {inv.account?.name ?? `Account #${inv.account_id}`}
                            </Link>
                          </TableCell>
                          <TableCell className="text-right font-mono tabular-nums">
                            {formatMoney(
                              inv.total_invested,
                              inv.currency,
                              hideBalances
                            )}
                          </TableCell>
                          <TableCell
                            className={`text-right font-medium font-mono tabular-nums ${
                              realizedPositive ? "text-positive" : "text-negative"
                            }`}
                          >
                            {realizedPositive ? "+" : ""}
                            <Money
                              amount={inv.realized_gain_loss}
                              currency={inv.currency}
                              baseAmount={inv.realized_gain_loss_base}
                            />
                          </TableCell>
                          <TableCell
                            className={`text-right font-medium font-mono tabular-nums ${
                              realizedPositive ? "text-positive" : "text-negative"
                            }`}
                          >
                            {realizedPositive ? "+" : ""}
                            {formatPercentage(Math.abs(pct))}
                          </TableCell>
                        </TableRow>
                      );
                    }
                    const f = holdingFigures(inv);
                    return (
                      <TableRow
                        key={inv.id}
                        className="cursor-pointer"
                        onClick={() => router.push(`/investments/${inv.id}`)}
                        onMouseEnter={() => prefetchInvestment(inv.id)}
                      >
                        <TableCell className="font-mono font-semibold">
                          <Link
                            href={`/investments/${inv.id}`}
                            onClick={(e) => e.stopPropagation()}
                          >
                            {inv.security.symbol}
                          </Link>
                        </TableCell>
                        <TableCell>{inv.security.name}</TableCell>
                        <TableCell>
                          <Link
                            href={`/accounts/${inv.account_id}`}
                            className="text-primary hover:underline"
                            onClick={(e) => e.stopPropagation()}
                          >
                            {inv.account?.name ?? `Account #${inv.account_id}`}
                          </Link>
                        </TableCell>
                        <TableCell className="text-right font-mono tabular-nums">
                          {inv.quantity.toFixed(
                            Number.isInteger(inv.quantity) ? 0 : 6
                          )}
                        </TableCell>
                        <TableCell className="text-right font-mono tabular-nums">
                          <Money
                            amount={inv.current_price_native}
                            currency={f.currency}
                            fxRate={inv.fx_rate}
                            fxStale={inv.fx_stale}
                            fxMissing={inv.fx_missing}
                          />
                        </TableCell>
                        <TableCell className="text-right font-medium font-mono">
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
                        <TableCell className="text-right font-medium font-mono">
                          <UnrealizedPnl investment={inv} />
                        </TableCell>
                        <TableCell
                          className={`text-right font-medium font-mono tabular-nums ${
                            realizedPositive ? "text-positive" : "text-negative"
                          }`}
                        >
                          {realizedPositive ? "+" : ""}
                          <Money
                            amount={inv.realized_gain_loss}
                            currency={inv.currency}
                            baseAmount={inv.realized_gain_loss_base}
                            className="justify-end"
                          />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>

            {totalPages > 1 && (
              <div className="mt-4 flex items-center justify-between">
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
      </CardContent>
    </Card>
  );
}

export default function InvestmentsPage() {
  const { data: portfolio, isLoading } = usePortfolio();
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();

  if (isLoading) {
    return <InvestmentsSkeleton />;
  }

  if (!portfolio) {
    return (
      <div className="space-y-6">
        <h1 className="font-display text-3xl leading-tight">Investments</h1>
        <Card>
          <CardHeader>
            <CardTitle>No Portfolio Data</CardTitle>
            <CardDescription>
              Add investments to your investment accounts to see your portfolio
              here.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Go to an{" "}
              <Link href="/accounts" className="text-primary underline">
                investment account
              </Link>{" "}
              and add your first investment to get started.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const portfolioCurrency = portfolio.currency || baseCurrency;
  const money = (cents: number) =>
    formatMoney(cents, portfolioCurrency, hideBalances);
  const signed = (cents: number) =>
    formatSigned(cents, portfolioCurrency, hideBalances);
  const gainUp = portfolio.total_gain_loss >= 0;
  const realizedUp = portfolio.total_realized_gain_loss >= 0;

  const holdingsCount = Object.values(portfolio.holdings_by_type).reduce(
    (sum, h) => sum + h.count,
    0
  );

  const holdingsEntries = Object.entries(portfolio.holdings_by_type).filter(
    ([, h]) => h.count > 0
  ) as [AssetType, { value: number; count: number }][];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-display text-3xl leading-tight">Investments</h1>
        <p className="text-sm text-muted-foreground">
          {holdingsCount} holding{holdingsCount !== 1 ? "s" : ""} across{" "}
          {holdingsEntries.length} asset class
          {holdingsEntries.length !== 1 ? "es" : ""}
        </p>
      </div>

      {/* Summary Cards (base currency) */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard
          label="Total value"
          value={money(portfolio.total_value)}
          icon={Wallet}
          tone="primary"
          delta={{
            text: `${gainUp ? "+" : ""}${formatPercentage(portfolio.gain_loss_pct)}`,
            direction: gainUp ? "up" : "down",
          }}
          sub="market value"
        />
        <StatCard
          label="Cost basis"
          value={money(portfolio.total_cost_basis)}
          icon={Coins}
          sub="Total invested"
        />
        <StatCard
          label="Unrealized G/L"
          value={signed(portfolio.total_gain_loss)}
          icon={gainUp ? TrendingUp : TrendingDown}
          tone={gainUp ? "positive" : "negative"}
          sub="open positions"
        />
        <StatCard
          label="Price return"
          value={signed(portfolio.total_price_return)}
          icon={portfolio.total_price_return >= 0 ? TrendingUp : TrendingDown}
          tone={portfolio.total_price_return >= 0 ? "positive" : "negative"}
          sub="from price movement"
        />
        <StatCard
          label="FX return"
          value={signed(portfolio.total_fx_return)}
          icon={Landmark}
          tone={portfolio.total_fx_return >= 0 ? "positive" : "negative"}
          sub="from currency movement"
        />
        <StatCard
          label="Realized G/L"
          value={signed(portfolio.total_realized_gain_loss)}
          icon={realizedUp ? TrendingUp : TrendingDown}
          tone={realizedUp ? "positive" : "negative"}
          sub="closed positions"
        />
      </div>

      <FxNotice
        unconverted={portfolio.unconverted}
        fxStale={portfolio.fx_stale}
      />

      {/* Value trend + allocation / currency exposure */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="min-w-0 lg:col-span-2">
          <InvestmentValueChart />
        </div>
        <div className="flex min-w-0 flex-col gap-4">
          <AssetAllocationChart
            holdingsByType={portfolio.holdings_by_type}
            totalValue={portfolio.total_value}
            currency={portfolioCurrency}
          />
          <CurrencyExposureChart />
        </div>
      </div>

      {/* All Holdings Table */}
      <AllHoldingsTable />
    </div>
  );
}
