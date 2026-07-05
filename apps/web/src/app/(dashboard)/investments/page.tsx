"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  TrendingUp,
  TrendingDown,
  Wallet,
  Coins,
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
  type InvestmentStatus,
} from "@/hooks/use-investments";
import { formatCurrency, formatPercentage } from "@/lib/format";
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

function returnPct(investment: Investment) {
  if (investment.total_invested === 0) return 0;
  return (investment.realized_gain_loss / investment.total_invested) * 100;
}

function HoldingCard({ investment, onClick }: { investment: Investment; onClick: () => void }) {
  const marketValue = Math.round(investment.quantity * investment.current_price);
  const unrealizedGL = marketValue - investment.cost_basis;
  const isUnrealizedPositive = unrealizedGL >= 0;
  const isRealizedPositive = investment.realized_gain_loss >= 0;

  return (
    <Card className="cursor-pointer transition-colors hover:bg-accent/50" onClick={onClick}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2 overflow-hidden">
          <div className="min-w-0 flex-shrink overflow-hidden max-w-[50%] sm:max-w-[60%]">
            <CardTitle className="text-base font-mono font-semibold truncate">
              {investment.security.symbol}
            </CardTitle>
            <p className="text-sm text-muted-foreground truncate">
              {investment.security.name}
            </p>
          </div>
          <div className="text-right flex-shrink-0 min-w-0 overflow-hidden">
            <p className="text-sm sm:text-base font-semibold font-mono tabular-nums truncate">
              {formatCurrency(marketValue)}
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
            <p className="font-medium font-mono tabular-nums truncate">
              {formatCurrency(investment.current_price)}
            </p>
          </div>
        </div>
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Unrealized G/L</span>
          <span
            className={`font-medium font-mono tabular-nums truncate ${
              isUnrealizedPositive ? "text-positive" : "text-negative"
            }`}
          >
            {isUnrealizedPositive ? "+" : ""}
            {formatCurrency(unrealizedGL)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Realized G/L</span>
          <span
            className={`font-medium font-mono tabular-nums truncate ${
              isRealizedPositive ? "text-positive" : "text-negative"
            }`}
          >
            {isRealizedPositive ? "+" : ""}
            {formatCurrency(investment.realized_gain_loss)}
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
}: {
  investment: Investment;
  onClick: () => void;
}) {
  const isRealizedPositive = investment.realized_gain_loss >= 0;
  const pct = returnPct(investment);

  return (
    <Card className="cursor-pointer transition-colors hover:bg-accent/50" onClick={onClick}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2 overflow-hidden">
          <div className="min-w-0 flex-shrink overflow-hidden max-w-[55%] sm:max-w-[65%]">
            <CardTitle className="text-base font-mono font-semibold truncate">
              {investment.security.symbol}
            </CardTitle>
            <p className="text-sm text-muted-foreground truncate">
              {investment.security.name}
            </p>
          </div>
          <div className="text-right flex-shrink-0 min-w-0 overflow-hidden">
            <p
              className={`text-sm sm:text-base font-semibold font-mono tabular-nums truncate ${
                isRealizedPositive ? "text-positive" : "text-negative"
              }`}
            >
              {isRealizedPositive ? "+" : ""}
              {formatCurrency(investment.realized_gain_loss)}
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 overflow-hidden">
        <div className="flex items-center justify-between gap-2 text-sm min-w-0">
          <span className="text-muted-foreground shrink-0">Invested</span>
          <span className="font-medium font-mono tabular-nums truncate">
            {formatCurrency(investment.total_invested)}
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
          aVal = a.current_price;
          bVal = b.current_price;
          break;
        case "market_value":
          aVal = Math.round(a.quantity * a.current_price);
          bVal = Math.round(b.quantity * b.current_price);
          break;
        case "unrealized_gl":
          aVal =
            Math.round(a.quantity * a.current_price) - a.cost_basis;
          bVal =
            Math.round(b.quantity * b.current_price) - b.cost_basis;
          break;
        case "realized_gl":
          aVal = a.realized_gain_loss;
          bVal = b.realized_gain_loss;
          break;
        case "total_invested":
          aVal = a.total_invested;
          bVal = b.total_invested;
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
                  />
                ) : (
                  <HoldingCard
                    key={inv.id}
                    investment={inv}
                    onClick={() => router.push(`/investments/${inv.id}`)}
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
                        >
                          <TableCell className="font-mono font-semibold">
                            {inv.security.symbol}
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
                            {formatCurrency(inv.total_invested)}
                          </TableCell>
                          <TableCell
                            className={`text-right font-medium font-mono tabular-nums ${
                              realizedPositive ? "text-positive" : "text-negative"
                            }`}
                          >
                            {realizedPositive ? "+" : ""}
                            {formatCurrency(inv.realized_gain_loss)}
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
                    const marketValue = Math.round(
                      inv.quantity * inv.current_price
                    );
                    const gainLoss = marketValue - inv.cost_basis;
                    const isPositive = gainLoss >= 0;
                    return (
                      <TableRow
                        key={inv.id}
                        className="cursor-pointer"
                        onClick={() => router.push(`/investments/${inv.id}`)}
                      >
                        <TableCell className="font-mono font-semibold">
                          {inv.security.symbol}
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
                          {formatCurrency(inv.current_price)}
                        </TableCell>
                        <TableCell className="text-right font-medium font-mono tabular-nums">
                          {formatCurrency(marketValue)}
                        </TableCell>
                        <TableCell
                          className={`text-right font-medium font-mono tabular-nums ${
                            isPositive ? "text-positive" : "text-negative"
                          }`}
                        >
                          {isPositive ? "+" : ""}
                          {formatCurrency(gainLoss)}
                        </TableCell>
                        <TableCell
                          className={`text-right font-medium font-mono tabular-nums ${
                            realizedPositive ? "text-positive" : "text-negative"
                          }`}
                        >
                          {realizedPositive ? "+" : ""}
                          {formatCurrency(inv.realized_gain_loss)}
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

  if (isLoading) {
    return <InvestmentsSkeleton />;
  }

  if (!portfolio) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Investments</h1>
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
        <h1 className="text-2xl font-semibold tracking-tight">Investments</h1>
        <p className="text-sm text-muted-foreground">
          {holdingsCount} holding{holdingsCount !== 1 ? "s" : ""} across{" "}
          {holdingsEntries.length} asset class
          {holdingsEntries.length !== 1 ? "es" : ""}
        </p>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Total value"
          value={formatCurrency(portfolio.total_value)}
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
          value={formatCurrency(portfolio.total_cost_basis)}
          icon={Coins}
          sub="Total invested"
        />
        <StatCard
          label="Unrealized G/L"
          value={`${gainUp ? "+" : "−"}${formatCurrency(Math.abs(portfolio.total_gain_loss))}`}
          icon={gainUp ? TrendingUp : TrendingDown}
          tone={gainUp ? "positive" : "negative"}
          sub="open positions"
        />
        <StatCard
          label="Realized G/L"
          value={`${realizedUp ? "+" : "−"}${formatCurrency(Math.abs(portfolio.total_realized_gain_loss))}`}
          icon={realizedUp ? TrendingUp : TrendingDown}
          tone={realizedUp ? "positive" : "negative"}
          sub="closed positions"
        />
      </div>

      {/* Value trend + allocation — equal-height row */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="min-w-0 lg:col-span-2">
          <InvestmentValueChart />
        </div>
        <AssetAllocationChart
          holdingsByType={portfolio.holdings_by_type}
          totalValue={portfolio.total_value}
        />
      </div>

      {/* All Holdings Table */}
      <AllHoldingsTable />
    </div>
  );
}
