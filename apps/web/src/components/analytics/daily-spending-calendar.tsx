"use client";

import { useMemo } from "react";
import Link from "next/link";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useDailySummary } from "@/hooks/use-transactions";
import { formatCompactAmount, toLocalDateString } from "@/lib/format";
import { heatIntensity, transactionsHref } from "@/lib/insights";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";
import { cn } from "@/lib/utils";

const WEEKDAY_LABELS = ["S", "M", "T", "W", "T", "F", "S"];

function parseLocalDate(str: string): Date {
  const [y, m, d] = str.split("-").map(Number);
  return new Date(y, m - 1, d);
}

function dateKey(year: number, month: number, day: number): string {
  return toLocalDateString(new Date(year, month, day));
}

/** Calendar months (0-based month) that the inclusive range touches. */
function spanningMonths(from: string, to: string) {
  const a = parseLocalDate(from);
  const b = parseLocalDate(to);
  const out: { year: number; month: number }[] = [];
  let y = a.getFullYear();
  let m = a.getMonth();
  while (y < b.getFullYear() || (y === b.getFullYear() && m <= b.getMonth())) {
    out.push({ year: y, month: m });
    m++;
    if (m > 11) {
      m = 0;
      y++;
    }
  }
  return out;
}

/**
 * Daily spending as a calendar heatmap: darker cells are bigger spending
 * days, with the compact amount in each cell. The cells grow to fill the
 * card's height, so the calendar matches the card beside it. Every day with spending links
 * to that day's expenses in Transactions.
 */
export function DailySpendingCalendar({
  fromDate,
  toDate,
  hideBalances,
}: {
  fromDate: string;
  toDate: string;
  hideBalances: boolean;
}) {
  const { data, isLoading, isError } = useDailySummary(fromDate, toDate);
  const baseCurrency = useBaseCurrency();
  const currency = data?.[0]?.currency ?? baseCurrency;
  const anyFxMissing = (data ?? []).some((d) => d.fx_missing);

  const spendByDate = useMemo(() => {
    const map = new Map<string, number>();
    for (const item of data ?? []) map.set(item.date, item.expenses);
    return map;
  }, [data]);

  const months = useMemo(
    () => spanningMonths(fromDate, toDate),
    [fromDate, toDate]
  );
  const multi = months.length > 1;
  const todayKey = toLocalDateString();

  const noSpendDays = useMemo(() => {
    let count = 0;
    const end = parseLocalDate(toDate < todayKey ? toDate : todayKey);
    for (
      let d = parseLocalDate(fromDate);
      d <= end;
      d = new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1)
    ) {
      if (!((spendByDate.get(toLocalDateString(d)) ?? 0) > 0)) count++;
    }
    return count;
  }, [fromDate, toDate, todayKey, spendByDate]);

  if (isLoading) {
    return (
      <Card aria-busy="true">
        <CardHeader>
          <CardTitle className="text-base">Daily spending</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[300px] w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Daily spending</CardTitle>
        <CardDescription>
          Amounts in {currency}. Click a day to see what you spent.
        </CardDescription>
        <FxNotice fxMissing={anyFxMissing} variant="inline" className="mt-1" />
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-5">
        {isError ? (
          <p role="alert" className="text-muted-foreground py-8 text-center text-sm">
            Couldn&apos;t load daily spending.
          </p>
        ) : (
          months.map(({ year, month }) => {
            const daysInMonth = new Date(year, month + 1, 0).getDate();
            const lead = new Date(year, month, 1).getDay();
            const label = new Date(year, month, 1).toLocaleDateString("en-US", {
              month: "long",
              year: "numeric",
            });
            return (
              <div key={`${year}-${month}`} className="flex flex-1 flex-col">
                {multi && (
                  <p className="text-muted-foreground mb-2 text-xs font-medium">
                    {label}
                  </p>
                )}
                <div className="text-muted-foreground mb-1 grid grid-cols-7 gap-1 text-center text-[10px] font-medium">
                  {WEEKDAY_LABELS.map((w, i) => (
                    <span key={i} aria-hidden>
                      {w}
                    </span>
                  ))}
                </div>
                <div className="grid flex-1 auto-rows-[minmax(2.5rem,1fr)] grid-cols-7 gap-1">
                  {Array.from({ length: lead }).map((_, i) => (
                    <span key={`lead-${i}`} />
                  ))}
                  {Array.from({ length: daysInMonth }).map((_, i) => {
                    const day = i + 1;
                    const key = dateKey(year, month, day);
                    const inRange = key >= fromDate && key <= toDate;
                    const cents = inRange ? (spendByDate.get(key) ?? 0) : 0;
                    const alpha = heatIntensity(cents);
                    const isToday = key === todayKey;
                    const cell = (
                      <>
                        <b className="text-[11px] leading-none font-medium">
                          {day}
                        </b>
                        {cents > 0 && !hideBalances && (
                          <em className="money mt-0.5 text-[10px] leading-none not-italic">
                            {formatCompactAmount(cents)}
                          </em>
                        )}
                      </>
                    );
                    const className = cn(
                      "flex min-w-0 flex-col items-center justify-center rounded-md border border-transparent p-0.5",
                      !inRange && "opacity-25",
                      isToday && "border-primary/60",
                      cents > 0 ? "text-foreground" : "text-muted-foreground"
                    );
                    const style = {
                      backgroundColor:
                        cents > 0
                          ? `color-mix(in oklab, var(--primary) ${Math.round(alpha * 55)}%, transparent)`
                          : "var(--muted)",
                    };
                    if (cents > 0) {
                      return (
                        <Link
                          key={key}
                          href={transactionsHref({
                            from: key,
                            to: key,
                            type: "expense",
                          })}
                          className={cn(
                            className,
                            "focus-visible:ring-ring/50 outline-none hover:brightness-110 focus-visible:ring-[3px]"
                          )}
                          style={style}
                          aria-label={`${label.split(" ")[0]} ${day}: spent ${(cents / 100).toFixed(2)} ${currency}`}
                        >
                          {cell}
                        </Link>
                      );
                    }
                    return (
                      <span key={key} className={className} style={style}>
                        {cell}
                      </span>
                    );
                  })}
                </div>
              </div>
            );
          })
        )}
        <div className="text-muted-foreground flex items-center gap-2 text-xs">
          <span>Less</span>
          <span
            aria-hidden
            className="h-2 w-16 rounded-full"
            style={{
              background:
                "linear-gradient(90deg, color-mix(in oklab, var(--primary) 8%, transparent), color-mix(in oklab, var(--primary) 55%, transparent))",
            }}
          />
          <span>More</span>
          <span className="ml-auto">
            {noSpendDays} no-spend {noSpendDays === 1 ? "day" : "days"}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
