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

// `fromDate` is a real UTC instant (from useActiveMonth's toISOString()), not
// a bare calendar-date string, so it must go through the Date constructor and
// local getters to recover the correct local month — naively slicing the "T"
// and reusing the UTC date parts shifts the month for any non-UTC timezone.
function localMonthAnchor(iso: string): Date {
  return new Date(iso);
}

function signedCompact(cents: number, sign: "+" | "-"): string {
  return `${sign}${formatCompact(cents / 100)}`;
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

  const monthAnchor = localMonthAnchor(fromDate);
  const year = monthAnchor.getFullYear();
  const month = monthAnchor.getMonth();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const firstWeekday = new Date(year, month, 1).getDay();

  const cells = useMemo(() => {
    const list: ({ day: number; dateKey: string } | null)[] = [];
    for (let i = 0; i < firstWeekday; i++) list.push(null);
    for (let day = 1; day <= daysInMonth; day++) {
      const dateKey = `${year}-${String(month + 1).padStart(2, "0")}-${String(
        day
      ).padStart(2, "0")}`;
      list.push({ day, dateKey });
    }
    while (list.length % 7 !== 0) list.push(null);
    return list;
  }, [firstWeekday, daysInMonth, year, month]);

  const today = new Date();
  const isCurrentCalendarMonth =
    today.getFullYear() === year && today.getMonth() === month;

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
          Income and expenses by day this month
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-7 gap-1 text-center text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          {WEEKDAY_LABELS.map((w) => (
            <div key={w} className="py-1">
              {w}
            </div>
          ))}
        </div>
        <div className="grid grid-cols-7 gap-1">
          {cells.map((cell, i) => {
            if (!cell) {
              return <div key={`blank-${i}`} className="aspect-square" />;
            }

            const entry = byDate.get(cell.dateKey);
            const income = entry?.income ?? 0;
            const expenses = entry?.expenses ?? 0;
            const hasActivity = income > 0 || expenses > 0;
            const isToday =
              isCurrentCalendarMonth && cell.day === today.getDate();

            return (
              <div
                key={cell.dateKey}
                className={cn(
                  "flex aspect-square flex-col items-center justify-center gap-0.5 rounded-lg border p-1 text-center",
                  hasActivity
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
                {hideBalances ? (
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
                )}
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
