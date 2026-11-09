"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ChevronLeft, TrendingDown, TrendingUp } from "lucide-react";

import { useSecurity, useSecurityPriceSeries } from "@/hooks/use-securities";
import { useAllInvestments } from "@/hooks/use-investments";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useAuth } from "@/hooks/use-auth";
import { convertCents, useFxLatest } from "@/hooks/use-fx-rates";
import { formatDate, formatMoney } from "@/lib/format";
import { assetTypeLabel } from "@/lib/asset-types";
import {
  PRICE_RANGES,
  changePct,
  priceRange,
  rangeDates,
  signedPercent,
  toPricePoints,
  type PriceRange,
} from "@/lib/price-series";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import { holdingFigures } from "@/components/investments/holding-figures";
import type { Security } from "@/types/models";

function BackLink() {
  return (
    <Link
      href="/securities"
      className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 -ml-1 inline-flex min-h-9 items-center gap-0.5 rounded-md px-1 text-sm outline-none focus-visible:ring-[3px]"
    >
      <ChevronLeft className="size-4" />
      Securities
    </Link>
  );
}

function SecurityDetailSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-6 w-28" />
      <div className="space-y-2">
        <Skeleton className="h-9 w-72" />
        <Skeleton className="h-4 w-56" />
      </div>
      <Skeleton className="h-[400px] w-full rounded-card" />
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-48 rounded-card" />
        <Skeleton className="h-48 rounded-card" />
      </div>
    </div>
  );
}

function KeyValue({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2.5">
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="text-right text-sm font-medium">{children}</dd>
    </div>
  );
}

/** Extra reference fields for the type of security (bond terms, crypto network, ...). */
function typeDetails(security: Security): { label: string; value: string }[] {
  const type = security.asset_type.toLowerCase();
  const rows: { label: string; value: string }[] = [];
  if (type === "bond") {
    if (security.maturity_date) {
      rows.push({ label: "Maturity date", value: formatDate(security.maturity_date) });
    }
    if (security.yield_to_maturity != null) {
      rows.push({
        label: "Yield to maturity",
        value: `${security.yield_to_maturity.toFixed(2)}%`,
      });
    }
    if (security.coupon_rate != null) {
      rows.push({ label: "Coupon rate", value: `${security.coupon_rate.toFixed(2)}%` });
    }
  }
  if (type === "crypto" && security.network) {
    rows.push({ label: "Network", value: security.network });
  }
  if (type === "reit" && security.property_type) {
    rows.push({ label: "Property type", value: security.property_type });
  }
  return rows;
}

export default function SecurityDetailPage() {
  const params = useParams();
  const securityId = params.id as string;
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const [range, setRange] = useState<PriceRange>("1Y");

  const {
    data: security,
    isLoading,
    isError,
    refetch,
  } = useSecurity(securityId);

  // One fetch of the whole history; the range buttons slice it on the client.
  const all = useMemo(() => rangeDates("ALL"), []);
  const {
    data: prices,
    isLoading: pricesLoading,
    isError: pricesError,
  } = useSecurityPriceSeries(securityId, all.from, all.to);
  const points = useMemo(() => toPricePoints(prices ?? []), [prices]);

  const inRange = useMemo(() => {
    if (range === "ALL") return points;
    const { from } = rangeDates(range);
    return points.filter((p) => p.day >= from);
  }, [points, range]);
  const yearRange = useMemo(() => {
    const { from } = rangeDates("1Y");
    return priceRange(points.filter((p) => p.day >= from));
  }, [points]);

  const data = useMemo(
    () => inRange.map((p) => ({ label: formatDate(p.day), value: p.cents })),
    [inRange]
  );
  const latest = points[points.length - 1];
  const change = changePct(inRange);

  const foreign = !!security && security.currency !== baseCurrency;
  const fx = useFxLatest(
    foreign ? security.currency : undefined,
    foreign ? baseCurrency : undefined
  );

  const { data: held } = useAllInvestments({
    page: 1,
    page_size: 100,
    status: "open",
  });
  const positions = useMemo(
    () => (held?.data ?? []).filter((h) => h.security_id === securityId),
    [held?.data, securityId]
  );

  if (isLoading) return <SecurityDetailSkeleton />;

  if (!security) {
    return (
      <div className="space-y-5">
        <BackLink />
        <div className="flex flex-col items-center justify-center gap-3 rounded-card border border-dashed p-12 text-center">
          <h1 className="text-lg font-semibold">
            {isError ? "Couldn't load this security" : "Security not found"}
          </h1>
          <p className="text-muted-foreground text-sm">
            {isError
              ? "Something went wrong while loading it."
              : "It does not exist or has been removed."}
          </p>
          {isError ? (
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              Try again
            </Button>
          ) : (
            <Button variant="outline" size="sm" asChild>
              <Link href="/securities">Back to Securities</Link>
            </Button>
          )}
        </div>
      </div>
    );
  }

  const currency = security.currency;
  const eyebrow = [
    security.symbol,
    assetTypeLabel(security.asset_type),
    security.exchange,
    foreign ? `${currency}-priced, shown in ${baseCurrency}` : currency,
  ].filter(Boolean);
  const baseAmount =
    latest && foreign ? convertCents(latest.cents, fx.data?.rate) : null;
  const up = (change ?? 0) >= 0;
  const sinceDay = points[0]?.day;
  const fullYear =
    !!sinceDay && Date.now() - Date.parse(sinceDay) >= 350 * 24 * 3600 * 1000;
  const providerSymbol =
    security.provider_symbol && security.provider_symbol !== security.symbol
      ? security.provider_symbol
      : null;

  return (
    <div className="space-y-5">
      <BackLink />

      <div className="space-y-1">
        <h1 className="font-display text-3xl leading-tight">{security.name}</h1>
        <p className="text-muted-foreground text-sm">{eyebrow.join(" · ")}</p>
      </div>

      <Card className="gap-4">
        <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="space-y-2">
            {latest ? (
              <>
                <Money amount={latest.cents} currency={currency} size="hero" dimCents />
                <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
                  {change !== null && (
                    <Badge tone={up ? "pos" : "neg"}>
                      {up ? <TrendingUp aria-hidden /> : <TrendingDown aria-hidden />}
                      {signedPercent(change)} · {range === "ALL" ? "all time" : range}
                    </Badge>
                  )}
                  <span className="text-muted-foreground text-sm">
                    updated {formatDate(latest.day)}
                  </span>
                </div>
                {foreign && baseAmount !== null && (
                  <p className="text-muted-foreground text-sm">
                    ≈ <Money amount={baseAmount} currency={baseCurrency} /> at 1{" "}
                    {currency} = {fx.data?.rate} {baseCurrency}
                    {fx.data?.stale && " (rate is out of date)"}
                  </p>
                )}
                {foreign && fx.isError && (
                  <p className="text-warning text-sm">
                    No exchange rate for {currency} to {baseCurrency}.
                  </p>
                )}
              </>
            ) : pricesLoading ? (
              <Skeleton className="h-14 w-56" />
            ) : (
              <p className="text-muted-foreground text-sm">No price recorded yet.</p>
            )}
          </div>
          <Segmented
            aria-label="Chart range"
            className="w-full md:w-auto"
            value={range}
            onValueChange={setRange}
            options={PRICE_RANGES.map((r) => ({ value: r, label: r === "ALL" ? "All" : r }))}
          />
        </CardHeader>
        <CardContent>
          {pricesLoading ? (
            <Skeleton className="h-[220px] w-full md:h-[260px]" />
          ) : pricesError ? (
            <p className="text-muted-foreground py-10 text-center text-sm">
              Couldn&apos;t load the price history. Try again in a moment.
            </p>
          ) : data.length === 0 ? (
            <p className="text-muted-foreground py-10 text-center text-sm">
              No price history for this range yet.
            </p>
          ) : (
            <ThemedAreaChart
              data={data}
              seriesLabel="Price"
              className="h-[220px] w-full md:h-[260px]"
              ditherColor="green"
              cleanColor="var(--chart-1)"
              yPadding={Math.max(
                1,
                Math.round(
                  (Math.max(...data.map((d) => d.value)) -
                    Math.min(...data.map((d) => d.value))) /
                    10
                )
              )}
              axisFormatter={(v) => formatMoney(v, currency, hideBalances)}
              valueFormatter={(v) => formatMoney(v, currency, hideBalances)}
            />
          )}
        </CardContent>
      </Card>

      <div className="grid gap-4 md:grid-cols-2">
        <Card className="gap-2">
          <CardHeader>
            <h2 className="font-semibold">Details</h2>
          </CardHeader>
          <CardContent>
            <dl className="divide-border/60 divide-y">
              {yearRange && (
                <KeyValue label={fullYear ? "52-week range" : `Range since ${formatDate(sinceDay ?? "")}`}>
                  <Money amount={yearRange.low} currency={currency} /> –{" "}
                  <Money amount={yearRange.high} currency={currency} />
                </KeyValue>
              )}
              <KeyValue label="Currency">{currency}</KeyValue>
              {security.exchange && (
                <KeyValue label="Exchange">{security.exchange}</KeyValue>
              )}
              {providerSymbol && (
                <KeyValue label="Provider symbol">
                  <span className="font-mono">{providerSymbol}</span>
                </KeyValue>
              )}
              {typeDetails(security).map((row) => (
                <KeyValue key={row.label} label={row.label}>
                  {row.value}
                </KeyValue>
              ))}
              {sinceDay && (
                <KeyValue label="Price points">
                  {points.length.toLocaleString()} since {formatDate(sinceDay)}
                </KeyValue>
              )}
            </dl>
          </CardContent>
        </Card>

        {positions.length > 0 && (
          <Card className="gap-2">
            <CardHeader>
              <h2 className="font-semibold">Your position</h2>
            </CardHeader>
            <CardContent className="divide-border/60 divide-y">
              {positions.map((p) => {
                const f = holdingFigures(p);
                const gain = f.basePnl ?? f.nativePnl;
                const gainCurrency = f.basePnl === null ? f.currency : baseCurrency;
                return (
                  <div
                    key={p.id}
                    className="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0"
                  >
                    <div className="min-w-0">
                      <p className="text-sm font-medium">
                        {p.quantity.toLocaleString(undefined, { maximumFractionDigits: 6 })}{" "}
                        units in {p.account?.name ?? `Account #${p.account_id}`}
                      </p>
                      <p className="text-muted-foreground text-xs">
                        Worth{" "}
                        <Money
                          amount={f.nativeValue}
                          currency={f.currency}
                          baseAmount={f.baseValue}
                          fxRate={p.fx_rate}
                          fxStale={p.fx_stale}
                          fxMissing={p.fx_missing}
                        />{" "}
                        · gain{" "}
                        <Money
                          amount={gain}
                          currency={gainCurrency}
                          sign="always"
                          fxMissing={f.basePnl === null}
                          className={gain >= 0 ? "text-positive" : "text-negative"}
                        />
                      </p>
                    </div>
                    <Button variant="outline" size="sm" asChild className="shrink-0">
                      <Link href={`/investments/${p.id}`}>Open holding</Link>
                    </Button>
                  </div>
                );
              })}
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
