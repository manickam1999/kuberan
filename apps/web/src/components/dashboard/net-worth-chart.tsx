"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ArrowDownRight, ArrowUpRight } from "lucide-react";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useIsMobile } from "@/hooks/use-mobile";
import { useReceivablesSummary } from "@/hooks/use-receivables";
import { LazyThemedAreaChart as ThemedAreaChart } from "@/components/charts/lazy";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/segmented";
import { Money } from "@/components/ui/money";
import { FxNotice } from "@/components/ui/fx-notice";
import { Eyebrow } from "@/components/dashboard/home-bits";
import {
  usePortfolioSnapshots,
  useGroupedPortfolioSnapshots,
} from "@/hooks/use-portfolio-snapshots";
import { useAuth } from "@/hooks/use-auth";
import {
  formatMoney,
  formatDate,
  formatTime,
  formatShortDateTime,
} from "@/lib/format";
import { balanceSheet } from "@/lib/home";
import type { PortfolioSnapshot } from "@/types/models";

interface PeriodOption {
  readonly value: string;
  readonly label: string;
  readonly months: number;
  readonly days: number;
  readonly groupBy: "day" | "hour" | undefined;
  readonly formatLabel: (iso: string) => string;
  /** Text after "since" / the whole caption when there is no start date. */
  readonly caption: string;
}

const PERIOD_OPTIONS: readonly PeriodOption[] = [
  {
    value: "1D",
    label: "1D",
    months: 0,
    days: 1,
    groupBy: undefined,
    formatLabel: formatTime,
    caption: "past day",
  },
  {
    value: "1W",
    label: "1W",
    months: 0,
    days: 7,
    groupBy: "hour",
    formatLabel: formatShortDateTime,
    caption: "past week",
  },
  {
    value: "1M",
    label: "1M",
    months: 1,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
    caption: "past month",
  },
  {
    value: "3M",
    label: "3M",
    months: 3,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
    caption: "past 3 months",
  },
  {
    value: "6M",
    label: "6M",
    months: 6,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
    caption: "past 6 months",
  },
  {
    value: "1Y",
    label: "1Y",
    months: 12,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
    caption: "past year",
  },
  {
    value: "ALL",
    label: "All",
    months: 120,
    days: 0,
    groupBy: "day",
    formatLabel: formatDate,
    caption: "all time",
  },
] as const;

/** Format a Date as YYYY-MM-DD using local date components (avoids UTC shift). */
function toLocalDateString(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function getDateRange(opt: PeriodOption) {
  const to = new Date();
  const from = new Date();
  if (opt.months > 0) {
    from.setMonth(from.getMonth() - opt.months);
  } else {
    from.setDate(from.getDate() - opt.days);
  }

  // Add 1 day to 'to' date to ensure we include all snapshots from the end date
  // (backend uses <= comparison with midnight UTC, so we need to go to next day)
  const toNextDay = new Date(to);
  toNextDay.setDate(toNextDay.getDate() + 1);

  return {
    from_date: toLocalDateString(from),
    to_date: toLocalDateString(toNextDay),
  };
}

/** Transform raw snapshot array into chart-ready data. */
function toChartData(
  snapshots: PortfolioSnapshot[],
  formatLabel: (iso: string) => string
) {
  return snapshots.map((s) => ({
    label: formatLabel(s.recorded_at),
    value: s.total_net_worth / 100,
  }));
}

/**
 * Periods tried, in order, until the user picks one. 6M is the default; a new
 * account has only a few days of snapshots, which 6M (grouped by day) shows as
 * a handful of points, so step down to finer grouping until the line has shape.
 */
const AUTO_PERIODS = ["6M", "1W", "1D"] as const;
const MIN_AUTO_POINTS = 8;

/** Room on the right so the endpoint dot and its halo are not clipped. */
const CHART_MARGIN = { top: 8, right: 12, bottom: 0, left: 12 } as const;

/** One line of the balance sheet: a swatch, a label and the amount. */
function SheetRow({
  swatch,
  label,
  amount,
  currency,
}: {
  swatch: string;
  label: string;
  amount: number;
  currency: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
        <i
          aria-hidden
          className="size-2 shrink-0 rounded-[2px]"
          style={{ backgroundColor: swatch }}
        />
        <span className="truncate">{label}</span>
      </span>
      <Money
        amount={amount}
        currency={currency}
        className="shrink-0 text-foreground"
      />
    </div>
  );
}

const CASH_COLOR = "var(--chart-1)";
const INVEST_COLOR = "var(--investment)";
const DEBT_COLOR = "var(--muted-foreground)";

/** Balance sheet and the owed-to-you line: the right half of the hero. */
function BalanceSheetPanel({
  snapshot,
  currency,
}: {
  snapshot: PortfolioSnapshot | undefined;
  currency: string;
}) {
  // Informational only: receivables are NOT part of net worth (plan 021, D4).
  const { data: receivables } = useReceivablesSummary();
  const owed = receivables?.total_outstanding ?? 0;
  const sheet = snapshot ? balanceSheet(snapshot) : null;
  const hasAssets = sheet !== null && sheet.cash + sheet.investments > 0;

  return (
    <div className="flex flex-col gap-4 border-t p-5 lg:border-t-0 lg:border-l">
      <Eyebrow>Balance sheet</Eyebrow>
      {sheet ? (
        <>
          <div
            role="img"
            aria-label="Cash and deposits compared with investments"
            className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted"
          >
            {hasAssets && (
              <>
                <i
                  className="h-full"
                  style={{
                    width: `${sheet.cashPct}%`,
                    backgroundColor: CASH_COLOR,
                  }}
                />
                <i
                  className="h-full flex-1"
                  style={{ backgroundColor: INVEST_COLOR }}
                />
              </>
            )}
          </div>
          <div className="space-y-2.5">
            <SheetRow
              swatch={CASH_COLOR}
              label="Cash & deposits"
              amount={sheet.cash}
              currency={currency}
            />
            <SheetRow
              swatch={INVEST_COLOR}
              label="Investments"
              amount={sheet.investments}
              currency={currency}
            />
            <SheetRow
              swatch={DEBT_COLOR}
              label="Credit & debt"
              amount={sheet.debt > 0 ? -sheet.debt : 0}
              currency={currency}
            />
          </div>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          The balance sheet appears once the first snapshot is recorded.
        </p>
      )}
      {owed > 0 && (
        <>
          <Link
            href="/receivables"
            className="mt-auto -mx-2 flex flex-col gap-0.5 rounded-lg border-t px-2 pt-3 transition-colors hover:bg-accent/40 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
          >
            <span className="flex items-center justify-between gap-3 text-sm">
              <span>Owed to you</span>
              <Money
                amount={owed}
                currency={receivables?.currency ?? currency}
                className="font-medium"
              />
            </span>
            <span className="text-xs text-muted-foreground">
              Not counted in net worth
            </span>
          </Link>
          <FxNotice fxMissing={receivables?.fx_missing} variant="inline" />
        </>
      )}
      {snapshot?.fx_missing && (
        <FxNotice fxMissing variant="inline" />
      )}
    </div>
  );
}

/** Home hero: net worth with range, delta and chart, beside the balance sheet. */
export function NetWorthChart() {
  const { hideBalances } = useAuth();
  const isMobile = useIsMobile();
  const baseCurrency = useBaseCurrency();
  // null until the user picks a period; until then AUTO_PERIODS decides.
  const [chosenPeriod, setChosenPeriod] = useState<string | null>(null);
  const [autoStep, setAutoStep] = useState(0);
  const period = chosenPeriod ?? AUTO_PERIODS[autoStep];

  const selectedOpt = useMemo(
    () =>
      PERIOD_OPTIONS.find((p) => p.value === period) ?? PERIOD_OPTIONS[4], // default 6M
    [period]
  );

  const { from_date, to_date } = useMemo(
    () => getDateRange(selectedOpt),
    [selectedOpt]
  );

  // For 1D (no group_by): fetch raw paginated snapshots with a large enough page_size.
  // 48 snapshots/day at 30-min intervals, pad a bit for safety.
  const rawQuery = usePortfolioSnapshots({
    from_date,
    to_date,
    page_size: 50,
  });

  // For 1W+ (with group_by): fetch downsampled snapshots.
  const groupedQuery = useGroupedPortfolioSnapshots({
    from_date,
    to_date,
    group_by: selectedOpt.groupBy,
  });

  const isGrouped = !!selectedOpt.groupBy;
  const activeQuery = isGrouped ? groupedQuery : rawQuery;
  const { isLoading, error } = activeQuery;

  // Snapshots oldest first, whichever query served them.
  const snapshots = useMemo<PortfolioSnapshot[]>(() => {
    if (isGrouped) return groupedQuery.data?.data ?? [];
    // Raw paginated response is newest first.
    return [...(rawQuery.data?.data ?? [])].reverse();
  }, [isGrouped, groupedQuery.data, rawQuery.data]);

  // The reporting currency can change, so snapshots may span currencies. The
  // chart plots the newest run of one currency. Earlier snapshots in another
  // currency are not charted, so the line never joins two currencies.
  const run = useMemo(() => {
    const last = snapshots[snapshots.length - 1];
    if (!last) return null;
    let start = snapshots.length - 1;
    while (start > 0 && snapshots[start - 1].currency === last.currency) {
      start--;
    }
    const before = start > 0 ? snapshots[start - 1] : null;
    return {
      currency: last.currency,
      points: snapshots.slice(start),
      changedAt: before ? snapshots[start].recorded_at : null,
      previousCurrency: before ? before.currency : null,
    };
  }, [snapshots]);

  const currency = run?.currency ?? baseCurrency;

  const chartData = useMemo(() => {
    if (!run) return [];
    return toChartData(run.points, selectedOpt.formatLabel);
  }, [run, selectedOpt.formatLabel]);

  useEffect(() => {
    if (chosenPeriod !== null || isLoading || error) return;
    if (chartData.length < MIN_AUTO_POINTS && autoStep < AUTO_PERIODS.length - 1) {
      setAutoStep((s) => s + 1);
    }
  }, [chosenPeriod, isLoading, error, chartData.length, autoStep]);

  const latestPoint = run?.points[run.points.length - 1];
  const latestNetWorth = latestPoint ? latestPoint.total_net_worth : null;

  // Change over the selected window (first vs last point), in cents.
  const periodChange = useMemo(() => {
    if (chartData.length < 2 || latestNetWorth === null) return null;
    const startCents = chartData[0].value * 100;
    const deltaCents = latestNetWorth - startCents;
    const pct = startCents !== 0 ? (deltaCents / Math.abs(startCents)) * 100 : 0;
    return { deltaCents, pct };
  }, [chartData, latestNetWorth]);

  // "since Jun 1, 2026" for day-grouped ranges; the range name otherwise.
  const sinceText =
    run && run.points.length > 0 && selectedOpt.groupBy === "day"
      ? `since ${formatDate(run.points[0].recorded_at)}`
      : selectedOpt.caption;

  if (isLoading) {
    return (
      <Card className="gap-0 overflow-hidden p-0 lg:grid lg:grid-cols-[minmax(0,1fr)_300px]">
        <div className="space-y-4 p-5">
          <div className="flex items-center justify-between">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-8 w-44" />
          </div>
          <Skeleton className="h-12 w-60" />
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-[120px] w-full md:h-[230px]" />
        </div>
        <div className="space-y-4 border-t p-5 lg:border-t-0 lg:border-l">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-2.5 w-full" />
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-5 w-full" />
        </div>
      </Card>
    );
  }

  const changeUp = periodChange ? periodChange.deltaCents >= 0 : true;

  return (
    <Card className="surface-glow gap-0 overflow-hidden p-0 lg:grid lg:grid-cols-[minmax(0,1fr)_300px]">
      <div className="min-w-0 space-y-3 p-5">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <Eyebrow>Net worth</Eyebrow>
          <Segmented
            aria-label="Net worth range"
            value={period}
            onValueChange={setChosenPeriod}
            options={PERIOD_OPTIONS.map((o) => ({
              value: o.value,
              label: o.label,
            }))}
          />
        </div>

        {latestNetWorth !== null ? (
          <Money
            amount={latestNetWorth}
            currency={currency}
            size="hero"
            className="block"
          />
        ) : (
          <span className="font-figures text-[40px] leading-none text-muted-foreground">
            —
          </span>
        )}

        {periodChange && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span
              className={`inline-flex h-[22px] items-center gap-1 rounded-full px-2 text-[11.5px] font-medium ${
                changeUp
                  ? "bg-positive-muted text-positive-text"
                  : "bg-negative-muted text-negative"
              }`}
            >
              {changeUp ? (
                <ArrowUpRight className="size-3" aria-hidden />
              ) : (
                <ArrowDownRight className="size-3" aria-hidden />
              )}
              <Money
                amount={periodChange.deltaCents}
                currency={currency}
                sign="always"
              />
              <span aria-hidden>·</span>
              <span className="tabular-nums">
                {hideBalances
                  ? "**%"
                  : `${changeUp ? "+" : "−"}${Math.abs(periodChange.pct).toFixed(1)}%`}
              </span>
            </span>
            <span className="text-xs text-muted-foreground">{sinceText}</span>
          </div>
        )}

        <div className="pt-1">
          {error ? (
            <div className="flex flex-col items-center justify-center py-10 text-destructive">
              <p className="text-sm">
                Could not load net worth history:{" "}
                {error instanceof Error ? error.message : "unknown error"}
              </p>
            </div>
          ) : chartData.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
              <p className="text-center text-sm">
                No history yet. Net worth snapshots are recorded by the price
                worker, so the chart fills in over the next day.
              </p>
            </div>
          ) : (
            <ThemedAreaChart
              data={chartData}
              seriesLabel="Net worth"
              className="h-[120px] w-full md:h-[230px]"
              ditherColor="green"
              cleanColor="var(--chart-1)"
              margin={CHART_MARGIN}
              hideXAxis={isMobile}
              valueFormatter={(v) => formatMoney(v * 100, currency, hideBalances)}
            />
          )}
        </div>

        {run?.changedAt && run.previousCurrency && (
          <p className="text-xs text-muted-foreground">
            Base currency changed to {run.currency} on{" "}
            {formatDate(run.changedAt)}. Earlier snapshots in{" "}
            {run.previousCurrency} are not charted, so the line does not join
            the two.
          </p>
        )}
      </div>

      <BalanceSheetPanel snapshot={latestPoint} currency={currency} />
    </Card>
  );
}
