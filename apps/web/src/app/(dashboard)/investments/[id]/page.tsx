"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { AlertTriangle, ChevronLeft, ChevronRight } from "lucide-react";

import {
  useInvestment,
  useInvestmentTransactions,
} from "@/hooks/use-investments";
import { useSecurityPriceSeries } from "@/hooks/use-securities";
import { formatDate, formatMoney } from "@/lib/format";
import { assetTypeLabel } from "@/lib/asset-types";
import {
  PRICE_RANGES,
  lastChangePct,
  rangeDates,
  signedPercent,
  toPricePoints,
  tradeMarkers,
  type PriceRange,
} from "@/lib/price-series";
import type { TradeKind } from "@/lib/trade-math";
import { Money } from "@/components/ui/money";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import { Segmented } from "@/components/ui/segmented";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import {
  UnrealizedPnl,
  holdingFigures,
} from "@/components/investments/holding-figures";
import { RecordTradeSheet } from "@/components/investments/record-trade-sheet";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import type {
  InvestmentTransaction,
  InvestmentTransactionType,
} from "@/types/models";
import { cn } from "@/lib/utils";

const TX_TYPE_CONFIG: Record<
  InvestmentTransactionType,
  { label: string; tone: "accent" | "pos" | "neg" | "neutral" | "warn" }
> = {
  buy: { label: "Buy", tone: "accent" },
  sell: { label: "Sell", tone: "neg" },
  dividend: { label: "Dividend", tone: "pos" },
  split: { label: "Split", tone: "neutral" },
  transfer: { label: "Transfer", tone: "warn" },
};

const PAGE_SIZE = 20;

function units(n: number): string {
  return n.toLocaleString(undefined, { maximumFractionDigits: 6 });
}

/** One line of the history: type chip, date, what happened, and the cash effect. */
function HistoryRow({ tx }: { tx: InvestmentTransaction }) {
  const config = TX_TYPE_CONFIG[tx.type];
  const isSplit = tx.type === "split";
  const isDividend = tx.type === "dividend";
  const isSell = tx.type === "sell";
  // Cash effect on the trade: out for a buy, in for a sell or dividend.
  const cashSigned = tx.type === "buy" ? -tx.total_amount : tx.total_amount;
  const cashBase =
    tx.base_amount === null
      ? null
      : tx.type === "buy"
        ? -tx.base_amount
        : tx.base_amount;

  return (
    <li className="flex items-start gap-3 px-4 py-3 md:px-5">
      <Badge tone={config.tone} className="mt-0.5 min-w-16 justify-center">
        {config.label}
      </Badge>
      <div className="min-w-0 flex-1">
        <p className="text-sm">{formatDate(tx.date)}</p>
        <p className="text-muted-foreground text-xs">
          {isSplit ? (
            <>{tx.split_ratio}:1 split</>
          ) : isDividend ? (
            <>
              {units(tx.quantity)} units
              {tx.fee > 0 && (
                <>
                  {" "}
                  · withheld{" "}
                  <Money amount={tx.fee} currency={tx.currency} />
                </>
              )}
            </>
          ) : (
            <>
              {units(tx.quantity)} ×{" "}
              <Money amount={tx.price_per_unit} currency={tx.currency} />
              {tx.fee > 0 && (
                <>
                  {" "}
                  · fee <Money amount={tx.fee} currency={tx.currency} />
                </>
              )}
            </>
          )}
        </p>
        {isSell && (
          <p className="text-xs">
            <span className="text-muted-foreground">Realised </span>
            <Money
              amount={tx.realized_gain_loss}
              currency={tx.currency}
              baseAmount={tx.realized_gain_loss_base}
              sign="always"
              className={tx.realized_gain_loss >= 0 ? "text-positive" : "text-negative"}
            />
          </p>
        )}
        {tx.notes && (
          <p className="text-muted-foreground mt-0.5 truncate text-xs">{tx.notes}</p>
        )}
      </div>
      {!isSplit && (
        <span className="shrink-0 text-right text-sm font-medium">
          <Money
            amount={cashSigned}
            currency={tx.currency}
            baseAmount={cashBase}
            fxRate={tx.fx_rate_to_base}
            sign="always"
            layout="stacked"
            className={cn("items-end", cashSigned > 0 && "text-positive")}
          />
        </span>
      )}
    </li>
  );
}

function BackLink() {
  return (
    <Link
      href="/investments"
      className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 -ml-1 inline-flex min-h-9 items-center gap-0.5 rounded-md px-1 text-sm outline-none focus-visible:ring-[3px]"
    >
      <ChevronLeft className="size-4" />
      Investments
    </Link>
  );
}

function HoldingSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-6 w-28" />
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-2">
          <Skeleton className="h-9 w-64" />
          <Skeleton className="h-4 w-48" />
        </div>
        <Skeleton className="hidden h-9 w-56 md:block" />
      </div>
      <Skeleton className="h-[88px] w-full rounded-card" />
      <Skeleton className="h-[300px] w-full rounded-card" />
      <Skeleton className="h-[240px] w-full rounded-card" />
    </div>
  );
}

function PriceChart({
  securityId,
  currency,
  investmentId,
}: {
  securityId: string;
  currency: string;
  investmentId: string;
}) {
  const { hideBalances } = useAuth();
  const [range, setRange] = useState<PriceRange>("1Y");
  const { from, to } = useMemo(() => rangeDates(range), [range]);
  const { data: prices, isLoading, isError } = useSecurityPriceSeries(
    securityId,
    from,
    to
  );
  const { data: tradesPage } = useInvestmentTransactions(investmentId, {
    page: 1,
    page_size: 100,
  });

  const points = useMemo(() => toPricePoints(prices ?? []), [prices]);
  const data = useMemo(
    () => points.map((p) => ({ label: formatDate(p.day), value: p.cents })),
    [points]
  );
  const markers = useMemo(
    () =>
      tradeMarkers(tradesPage?.data ?? [], points).map((m) => ({
        label: formatDate(m.day),
        tone: m.tone,
      })),
    [tradesPage?.data, points]
  );

  return (
    <Card className="gap-3">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3">
        <span className="text-muted-foreground text-xs font-medium tracking-wider uppercase">
          Price · your trades marked
        </span>
        <Segmented
          aria-label="Chart range"
          value={range}
          onValueChange={setRange}
          options={PRICE_RANGES.map((r) => ({ value: r, label: r === "ALL" ? "All" : r }))}
        />
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[220px] w-full md:h-[260px]" />
        ) : isError ? (
          <p className="text-muted-foreground py-10 text-center text-sm">
            Couldn&apos;t load the price history. Try again in a moment.
          </p>
        ) : data.length === 0 ? (
          <p className="text-muted-foreground py-10 text-center text-sm">
            No price history for this range yet.
          </p>
        ) : (
          <>
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
              markers={markers}
            />
            {markers.length > 0 && (
              <p className="text-muted-foreground mt-2 flex items-center gap-3 text-xs">
                <span className="inline-flex items-center gap-1.5">
                  <i className="border-positive bg-card size-2.5 rounded-full border-2" />
                  Buy
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <i className="border-negative bg-card size-2.5 rounded-full border-2" />
                  Sell
                </span>
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

export default function InvestmentDetailPage() {
  const params = useParams();
  const investmentId = params.id as string;

  const baseCurrency = useBaseCurrency();
  const [txPage, setTxPage] = useState(1);
  const [tradeKind, setTradeKind] = useState<TradeKind | null>(null);

  const {
    data: investment,
    isLoading,
    isError,
    refetch,
  } = useInvestment(investmentId);
  const { data: txData, isLoading: txLoading } = useInvestmentTransactions(
    investmentId,
    { page: txPage, page_size: PAGE_SIZE }
  );
  // The last two prices give "today's" move for the Price figure.
  const { from, to } = useMemo(() => rangeDates("1M"), []);
  const { data: recentPrices } = useSecurityPriceSeries(
    investment?.security_id ?? "",
    from,
    to
  );
  const todayPct = useMemo(
    () => lastChangePct(toPricePoints(recentPrices ?? [])),
    [recentPrices]
  );

  const transactions = txData?.data ?? [];
  const txTotalPages = txData?.total_pages ?? 1;

  if (isLoading) return <HoldingSkeleton />;

  if (!investment) {
    return (
      <div className="space-y-5">
        <BackLink />
        <div className="flex flex-col items-center justify-center gap-3 rounded-card border border-dashed p-12 text-center">
          <h1 className="text-lg font-semibold">
            {isError ? "Couldn't load this holding" : "Investment not found"}
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
              <Link href="/investments">Back to Investments</Link>
            </Button>
          )}
        </div>
      </div>
    );
  }

  const isClosed = investment.quantity <= 0;
  const f = holdingFigures(investment);
  const currency = f.currency;
  // Headline gain is in the base currency (base value - base cost); without a
  // rate it falls back to the native gain (native value - native cost).
  const pnlInBase = f.basePnl !== null;
  const gainLoss = f.basePnl ?? f.nativePnl;
  const pnlCost = pnlInBase ? investment.cost_basis_base : investment.cost_basis;
  const gainLossPct = pnlCost > 0 ? (gainLoss / pnlCost) * 100 : 0;
  const costCurrency = pnlInBase ? baseCurrency : currency;
  const hasRealized = investment.realized_gain_loss !== 0;
  const realizedUp = investment.realized_gain_loss >= 0;
  const avgCost =
    investment.quantity > 0
      ? Math.round(investment.cost_basis / investment.quantity)
      : 0;

  const price: StatStripItem = {
    label: "Price",
    value: (
      <Money
        amount={investment.current_price_native}
        currency={currency}
        baseAmount={investment.current_price}
        fxRate={investment.fx_rate}
        fxStale={investment.fx_stale}
        fxMissing={investment.fx_missing}
        layout="stacked"
      />
    ),
    sub:
      todayPct === null ? undefined : (
        <span className={todayPct >= 0 ? "text-positive" : "text-negative"}>
          {signedPercent(todayPct)} today
        </span>
      ),
  };
  const realized: StatStripItem = {
    label: "Realised gain",
    tone: realizedUp ? "pos" : "neg",
    value: (
      <Money
        amount={investment.realized_gain_loss}
        currency={currency}
        baseAmount={investment.realized_gain_loss_base}
        sign="always"
        layout="stacked"
      />
    ),
    sub: "from sells",
  };

  const items: StatStripItem[] = isClosed
    ? [
        realized,
        {
          label: "Invested",
          value: (
            <Money amount={investment.total_invested} currency={currency} layout="stacked" />
          ),
          sub: "all buys",
        },
        price,
      ]
    : [
        {
          label: "Position value",
          value: (
            <Money
              amount={f.nativeValue}
              currency={currency}
              baseAmount={f.baseValue}
              fxRate={investment.fx_rate}
              fxStale={investment.fx_stale}
              fxMissing={investment.fx_missing}
              layout="stacked"
            />
          ),
        },
        {
          label: hasRealized ? "Unrealised gain" : "Gain",
          tone: gainLoss >= 0 ? "pos" : "neg",
          value: <UnrealizedPnl investment={investment} />,
          sub: (
            <>
              {signedPercent(gainLossPct)} on{" "}
              <Money amount={pnlCost} currency={costCurrency} />
            </>
          ),
        },
        {
          label: "Quantity",
          value: <span className="tabular-nums">{units(investment.quantity)}</span>,
          sub: (
            <>
              avg cost <Money amount={avgCost} currency={currency} />
            </>
          ),
        },
        price,
        ...(hasRealized ? [realized] : []),
      ];

  const eyebrow = [
    investment.security.symbol,
    assetTypeLabel(investment.security.asset_type),
    investment.security.exchange,
  ].filter(Boolean);

  return (
    <div className="space-y-5">
      <BackLink />

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h1 className="font-display text-3xl leading-tight">
            {investment.security.name}
          </h1>
          <p className="text-muted-foreground flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm">
            <span>{eyebrow.join(" · ")}</span>
            <span aria-hidden>·</span>
            <Link
              href={`/accounts/${investment.account_id}`}
              className="text-primary hover:underline"
            >
              {investment.account?.name ?? `Account #${investment.account_id}`}
            </Link>
            {isClosed && <Badge tone="neutral">Position closed</Badge>}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            className="hidden md:inline-flex"
            onClick={() => setTradeKind("dividend")}
          >
            Dividend
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="hidden md:inline-flex"
            onClick={() => setTradeKind("split")}
            disabled={isClosed}
          >
            Split
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setTradeKind("sell")}
            disabled={isClosed}
          >
            Sell
          </Button>
          <Button size="sm" onClick={() => setTradeKind("buy")}>
            Buy
          </Button>
        </div>
      </div>

      <StatStrip items={items} />
      {(investment.fx_missing || investment.fx_stale) && (
        <div
          role="status"
          className="border-warning/30 bg-warning-muted text-warning flex items-start gap-2 rounded-md border px-3 py-2 text-xs"
        >
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>
            {investment.fx_missing
              ? `No exchange rate for ${currency} to ${baseCurrency}, so figures are shown in ${currency} only.`
              : `The ${currency} to ${baseCurrency} exchange rate is out of date.`}
          </span>
        </div>
      )}

      <PriceChart
        securityId={investment.security_id}
        currency={currency}
        investmentId={investmentId}
      />

      <Card className="gap-0 py-0">
        <CardHeader className="flex flex-row items-center justify-between gap-3 border-b px-4 py-3 md:px-5">
          <h2 className="font-semibold">History</h2>
          <div className="flex gap-2 md:hidden">
            <Button variant="outline" size="sm" onClick={() => setTradeKind("dividend")}>
              Dividend
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setTradeKind("split")}
              disabled={isClosed}
            >
              Split
            </Button>
          </div>
        </CardHeader>
        {txLoading ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : transactions.length === 0 ? (
          <div className="flex flex-col items-center gap-3 p-10 text-center">
            <p className="text-muted-foreground text-sm">
              No trades recorded for this holding yet.
            </p>
            <Button size="sm" onClick={() => setTradeKind("buy")}>
              Record a buy
            </Button>
          </div>
        ) : (
          <>
            <ul className="divide-border/60 divide-y">
              {transactions.map((tx) => (
                <HistoryRow key={tx.id} tx={tx} />
              ))}
            </ul>
            {txTotalPages > 1 && (
              <div className="flex items-center justify-between border-t px-4 py-3 md:px-5">
                <span className="text-muted-foreground text-sm">
                  Page {txPage} of {txTotalPages}
                </span>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={txPage <= 1}
                    onClick={() => setTxPage((p) => p - 1)}
                  >
                    <ChevronLeft className="size-4" />
                    <span className="ml-1 hidden sm:inline">Previous</span>
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={txPage >= txTotalPages}
                    onClick={() => setTxPage((p) => p + 1)}
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

      <RecordTradeSheet
        open={tradeKind !== null}
        onOpenChange={(open) => {
          if (!open) setTradeKind(null);
        }}
        investmentId={investmentId}
        initialKind={tradeKind ?? "buy"}
      />
    </div>
  );
}
