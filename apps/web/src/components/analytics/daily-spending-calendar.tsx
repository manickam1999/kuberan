"use client";

import { useMemo } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useDailySummary } from "@/hooks/use-transactions";
import { formatCompact } from "@/lib/format";
import { cn } from "@/lib/utils";

const WEEKDAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/**
 * Parse a date string robustly as a LOCAL date, avoiding the UTC-midnight
 * off-by-one that `new Date("YYYY-MM-DD")` causes in timezones behind UTC.
 *
 * - If the string contains "T" it is treated as an ISO instant and handed
 *   straight to the Date constructor (which interprets "T"-bearing strings in
 *   the local timezone per spec).
 * - Otherwise it is split on "-" and built via `new Date(y, m-1, d)` so the
 *   result is always in local time regardless of the user's UTC offset.
 */
function parseLocalDate(str: string): Date {
  if (str.includes("T")) {
    return new Date(str);
  }
  const parts = str.split("-");
  const y = parseInt(parts[0], 10);
  const m = parseInt(parts[1], 10);
  const d = parseInt(parts[2], 10);
  return new Date(y, m - 1, d);
}

/**
 * Format a YYYY-MM-DD date key for a given year, 0-based month, and day.
 */
function toDateKey(year: number, month: number, day: number): string {
  return `${year}-${String(month + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

/**
 * Build the ordered list of { year, month } objects (month is 0-based) that
 * the given [fromDate, toDate] range spans, inclusive.
 */
function spanningMonths(
  fromDate: string,
  toDate: string
): { year: number; month: number }[] {
  const from = parseLocalDate(fromDate);
  const to = parseLocalDate(toDate);

  const startYear = from.getFullYear();
  const startMonth = from.getMonth();
  const endYear = to.getFullYear();
  const endMonth = to.getMonth();

  const result: { year: number; month: number }[] = [];
  let y = startYear;
  let m = startMonth;
  while (y < endYear || (y === endYear && m <= endMonth)) {
    result.push({ year: y, month: m });
    m++;
    if (m > 11) {
      m = 0;
      y++;
    }
  }
  return result;
}

function signedCompact(cents: number, sign: "+" | "-"): string {
  return `${sign}${formatCompact(cents / 100)}`;
}

/**
 * Build the ordered list of cells (null = blank leading/trailing pad, object =
 * real day) for a single calendar month grid.
 */
function buildMonthCells(
  year: number,
  month: number
): ({ day: number; dateKey: string } | null)[] {
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const firstWeekday = new Date(year, month, 1).getDay();
  const list: ({ day: number; dateKey: string } | null)[] = [];
  for (let i = 0; i < firstWeekday; i++) list.push(null);
  for (let day = 1; day <= daysInMonth; day++) {
    list.push({ day, dateKey: toDateKey(year, month, day) });
  }
  while (list.length % 7 !== 0) list.push(null);
  return list;
}

export function DailySpendingCalendar({
  fromDate,
  toDate,
  hideBalances,
}: {
  fromDate: string;
  toDate: string;
  hideBalances: boolean;
}) {
  const { data, isLoading } = useDailySummary(fromDate, toDate);

  const byDate = useMemo(() => {
    const map = new Map<string, { income: number; expenses: number }>();
    for (const item of data ?? []) {
      map.set(item.date, { income: item.income, expenses: item.expenses });
    }
    return map;
  }, [data]);

  // Compute the list of calendar months this range spans (stable across renders).
  const months = useMemo(
    () => spanningMonths(fromDate, toDate),
    [fromDate, toDate]
  );

  // Pre-compute per-month cell arrays inside useMemo to avoid re-building each render.
  const monthGrids = useMemo(
    () =>
      months.map(({ year, month }) => ({
        year,
        month,
        cells: buildMonthCells(year, month),
      })),
    [months]
  );

  const isMultiMonth = months.length > 1;

  // For "out of range" boundary checking we need Date objects.
  const rangeFrom = useMemo(() => parseLocalDate(fromDate), [fromDate]);
  const rangeTo = useMemo(() => parseLocalDate(toDate), [toDate]);

  const today = new Date();
  const todayYear = today.getFullYear();
  const todayMonth = today.getMonth();
  const todayDay = today.getDate();

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Daily activity</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[340px] w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Daily activity</CardTitle>
        <CardDescription>
          {isMultiMonth
            ? "Income and expenses by day"
            : "Income and expenses by day this month"}
        </CardDescription>
      </CardHeader>
      <CardContent className={cn(isMultiMonth && "space-y-6")}>
        {monthGrids.map(({ year, month, cells }) => {
          const isCurrentCalendarMonth =
            todayYear === year && todayMonth === month;

          // Month sub-heading for multi-month view.
          const monthLabel = new Date(year, month, 1).toLocaleDateString(
            "en-US",
            { month: "long", year: "numeric" }
          );

          return (
            <div key={`${year}-${month}`}>
              {isMultiMonth && (
                <p className="mb-2 text-xs font-medium text-muted-foreground">
                  {monthLabel}
                </p>
              )}
              {/* Weekday header */}
              <div className="grid grid-cols-7 gap-1 text-center text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                {WEEKDAY_LABELS.map((w) => (
                  <div key={w} className="py-1">
                    {w}
                  </div>
                ))}
              </div>
              {/* Day grid */}
              <div className="grid grid-cols-7 gap-1">
                {cells.map((cell, i) => {
                  if (!cell) {
                    return <div key={`blank-${i}`} className="aspect-square" />;
                  }

                  // Determine whether this day is within the selected range.
                  const cellDate = new Date(year, month, cell.day);
                  const inRange =
                    cellDate >= rangeFrom && cellDate <= rangeTo;

                  const entry = inRange ? byDate.get(cell.dateKey) : undefined;
                  const income = entry?.income ?? 0;
                  const expenses = entry?.expenses ?? 0;
                  const hasActivity = income > 0 || expenses > 0;
                  const isToday =
                    isCurrentCalendarMonth && cell.day === todayDay;

                  return (
                    <div
                      key={cell.dateKey}
                      className={cn(
                        "flex aspect-square flex-col items-center justify-center gap-0.5 rounded-lg border p-1 text-center",
                        !inRange && "opacity-30",
                        inRange && hasActivity
                          ? "border-transparent bg-muted/50"
                          : "border-transparent",
                        isToday && "border-primary/50"
                      )}
                    >
                      <span
                        className={cn(
                          "text-[11px]",
                          isToday
                            ? "font-semibold text-primary"
                            : "text-muted-foreground"
                        )}
                      >
                        {cell.day}
                      </span>
                      {inRange &&
                        (hideBalances ? (
                          <div className="flex gap-1">
                            {income > 0 && (
                              <span className="size-1.5 rounded-full bg-positive" />
                            )}
                            {expenses > 0 && (
                              <span className="size-1.5 rounded-full bg-negative" />
                            )}
                          </div>
                        ) : (
                          <div className="flex flex-col leading-tight">
                            {income > 0 && (
                              <span className="money text-[10px] font-medium text-positive">
                                {signedCompact(income, "+")}
                              </span>
                            )}
                            {expenses > 0 && (
                              <span className="money text-[10px] font-medium text-negative">
                                {signedCompact(expenses, "-")}
                              </span>
                            )}
                          </div>
                        ))}
                    </div>
                  );
                })}
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
