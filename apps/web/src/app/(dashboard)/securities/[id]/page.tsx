"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ChevronLeft, ChevronRight } from "lucide-react";

import { useSecurity, useSecurityPriceHistory } from "@/hooks/use-securities";
import { formatCurrency, formatDate } from "@/lib/format";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import type { AssetType } from "@/types/models";

const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  bond: "Bond",
  crypto: "Crypto",
  reit: "REIT",
  commodity: "Commodity",
};

type PricePeriod = "1M" | "3M" | "6M" | "1Y";

const PERIOD_MONTHS: Record<PricePeriod, number> = {
  "1M": 1,
  "3M": 3,
  "6M": 6,
  "1Y": 12,
};

function computeFromDate(period: PricePeriod): string {
  const now = new Date();
  now.setMonth(now.getMonth() - PERIOD_MONTHS[period]);
  return now.toISOString().split("T")[0];
}

function getTodayDate(): string {
  return new Date().toISOString().split("T")[0];
}

const PAGE_SIZE = 20;

function SecurityDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-6 w-32" />
      <div className="space-y-2">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-6 w-48" />
      </div>
      <div className="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
        <Skeleton className="h-24" />
        <Skeleton className="h-24" />
        <Skeleton className="h-24" />
      </div>
      <Skeleton className="h-64" />
    </div>
  );
}

export default function SecurityDetailPage() {
  const params = useParams();
  const securityId = params.id as string;

  const [pricePeriod, setPricePeriod] = useState<PricePeriod>("3M");
  const [pricePage, setPricePage] = useState(1);

  const fromDate = useMemo(() => computeFromDate(pricePeriod), [pricePeriod]);
  const toDate = useMemo(() => getTodayDate(), []);

  const { data: security, isLoading } = useSecurity(securityId);
  const { data: priceData, isLoading: pricesLoading } =
    useSecurityPriceHistory(securityId, fromDate, toDate, {
      page: pricePage,
      page_size: PAGE_SIZE,
    });

  const prices = priceData?.data ?? [];
  const priceTotalPages = priceData?.total_pages ?? 1;

  if (isLoading) {
    return <SecurityDetailSkeleton />;
  }

  if (!security) {
    return (
      <div className="space-y-6">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/securities">
            <ArrowLeft className="mr-1 h-4 w-4" />
            Back to Securities
          </Link>
        </Button>
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">Security not found</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            The security you are looking for does not exist or has been removed.
          </p>
        </div>
      </div>
    );
  }

  const assetType = security.asset_type.toLowerCase() as AssetType;

  return (
    <div className="space-y-6">
      {/* Back button */}
      <Button variant="ghost" size="sm" asChild>
        <Link href="/securities">
          <ArrowLeft className="mr-1 h-4 w-4" />
          Back to Securities
        </Link>
      </Button>

      {/* Header */}
      <div className="space-y-2">
        <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
          <h1 className="text-2xl font-bold font-mono">{security.symbol}</h1>
          <Badge variant="outline" className="self-start">
            {ASSET_TYPE_LABELS[assetType] ?? security.asset_type}
          </Badge>
        </div>
        <p className="text-lg text-muted-foreground">{security.name}</p>
      </div>

      {/* Info cards */}
      <div className="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Currency</CardDescription>
            <CardTitle className="text-lg">{security.currency}</CardTitle>
          </CardHeader>
        </Card>

        {security.exchange && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Exchange</CardDescription>
              <CardTitle className="text-lg">{security.exchange}</CardTitle>
            </CardHeader>
          </Card>
        )}

        {/* Bond-specific fields */}
        {assetType === "bond" && security.maturity_date && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Maturity Date</CardDescription>
              <CardTitle className="text-lg">
                {formatDate(security.maturity_date)}
              </CardTitle>
            </CardHeader>
          </Card>
        )}
        {assetType === "bond" && security.yield_to_maturity != null && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Yield to Maturity</CardDescription>
              <CardTitle className="text-lg">
                {security.yield_to_maturity.toFixed(2)}%
              </CardTitle>
            </CardHeader>
          </Card>
        )}
        {assetType === "bond" && security.coupon_rate != null && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Coupon Rate</CardDescription>
              <CardTitle className="text-lg">
                {security.coupon_rate.toFixed(2)}%
              </CardTitle>
            </CardHeader>
          </Card>
        )}

        {/* Crypto-specific fields */}
        {assetType === "crypto" && security.network && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Network</CardDescription>
              <CardTitle className="text-lg">{security.network}</CardTitle>
            </CardHeader>
          </Card>
        )}

        {/* REIT-specific fields */}
        {assetType === "reit" && security.property_type && (
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Property Type</CardDescription>
              <CardTitle className="text-lg">
                {security.property_type}
              </CardTitle>
            </CardHeader>
          </Card>
        )}
      </div>

      {/* Price History */}
      <Card>
        <CardHeader>
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div>
              <CardTitle>Price History</CardTitle>
              <CardDescription>
                Historical prices for {security.symbol}
              </CardDescription>
            </div>
            <Tabs
              value={pricePeriod}
              onValueChange={(v) => {
                setPricePeriod(v as PricePeriod);
                setPricePage(1);
              }}
            >
              <TabsList className="w-full sm:w-auto">
                <TabsTrigger value="1M" className="flex-1 sm:flex-initial">1M</TabsTrigger>
                <TabsTrigger value="3M" className="flex-1 sm:flex-initial">3M</TabsTrigger>
                <TabsTrigger value="6M" className="flex-1 sm:flex-initial">6M</TabsTrigger>
                <TabsTrigger value="1Y" className="flex-1 sm:flex-initial">1Y</TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
        </CardHeader>
        <CardContent>
          {pricesLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-10 w-full" />
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : prices.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
              <h3 className="text-lg font-semibold">No price data</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                No price history available for this period.
              </p>
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Date</TableHead>
                    <TableHead className="text-right">Price</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {prices.map((price) => (
                    <TableRow key={price.id}>
                      <TableCell>{formatDate(price.recorded_at)}</TableCell>
                      <TableCell className="text-right font-mono">
                        {formatCurrency(price.price, security.currency)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>

              {priceTotalPages > 1 && (
                <div className="mt-4 flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">
                    Page {pricePage} of {priceTotalPages}
                  </span>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={pricePage <= 1}
                      onClick={() => setPricePage((p) => p - 1)}
                    >
                      <ChevronLeft className="h-4 w-4" />
                      <span className="ml-1 hidden sm:inline">Previous</span>
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={pricePage >= priceTotalPages}
                      onClick={() => setPricePage((p) => p + 1)}
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
    </div>
  );
}
