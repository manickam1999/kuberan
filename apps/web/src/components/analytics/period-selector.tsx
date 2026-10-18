"use client";

import * as React from "react";
import { ChevronLeft, ChevronRight, Calendar, ChevronDown } from "lucide-react";
import type { DateRange } from "react-day-picker";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from "@/components/ui/dropdown-menu";
import { Calendar as CalendarPrimitive } from "@/components/ui/calendar";
import { toLocalDateString } from "@/lib/format";
import type { AnalyticsPeriod } from "@/hooks/use-analytics-period";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MONTH_NAMES = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

/** Parse a YYYY-MM-DD string into a LOCAL Date (not UTC). */
function parseLocalDate(ymd: string): Date {
  const [y, m, d] = ymd.split("-").map(Number);
  return new Date(y, m - 1, d);
}

/**
 * Return the year and 0-based month index from a YYYY-MM-DD string.
 * Used to extract the "current" month from period.fromDate.
 */
function yearMonthFromDate(ymd: string): { year: number; monthIndex: number } {
  const [y, m] = ymd.split("-").map(Number);
  return { year: y, monthIndex: m - 1 };
}

// ---------------------------------------------------------------------------
// Sub-component: MonthYearPicker popover
// ---------------------------------------------------------------------------

interface MonthYearPickerProps {
  /** Currently active fromDate (YYYY-MM-DD) — used to highlight the selected month. */
  fromDate: string;
  /** Whether the period is an exact calendar month (highlight the cell). */
  isMonthMode: boolean;
  onSelectMonth: (year: number, monthIndex: number) => void;
}

function MonthYearPicker({
  fromDate,
  isMonthMode,
  onSelectMonth,
}: MonthYearPickerProps) {
  const [open, setOpen] = React.useState(false);

  const { year: currentFromYear, monthIndex: currentFromMonth } =
    yearMonthFromDate(fromDate);

  // The year shown in the picker's stepper (independent state while the popover is open).
  const [pickerYear, setPickerYear] = React.useState(currentFromYear);

  // Sync pickerYear when the period changes externally.
  React.useEffect(() => {
    if (!open) {
      setPickerYear(currentFromYear);
    }
  }, [currentFromYear, open]);

  const handleSelect = (monthIndex: number) => {
    onSelectMonth(pickerYear, monthIndex);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="gap-1 font-medium"
          aria-label="Open month and year picker"
        >
          <Calendar className="size-3.5 text-muted-foreground" />
          <span>Month</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-3" align="start">
        {/* Year stepper */}
        <div className="mb-3 flex items-center justify-between">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setPickerYear((y) => y - 1)}
            aria-label="Previous year"
          >
            <ChevronLeft className="size-4" />
          </Button>
          <span className="text-sm font-semibold">{pickerYear}</span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setPickerYear((y) => y + 1)}
            disabled={
              pickerYear >=
              (() => {
                const now = new Date();
                return now.getFullYear();
              })()
            }
            aria-label="Next year"
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>

        {/* 3×4 month grid */}
        <div className="grid grid-cols-3 gap-1">
          {MONTH_NAMES.map((name, idx) => {
            const isSelected =
              isMonthMode &&
              pickerYear === currentFromYear &&
              idx === currentFromMonth;

            // Disable future months
            const now = new Date();
            const isFuture =
              pickerYear > now.getFullYear() ||
              (pickerYear === now.getFullYear() && idx > now.getMonth());

            return (
              <Button
                key={name}
                variant={isSelected ? "default" : "ghost"}
                size="sm"
                className={cn(
                  "h-8 text-xs",
                  !isSelected && "text-foreground"
                )}
                disabled={isFuture}
                onClick={() => handleSelect(idx)}
                aria-label={`${name} ${pickerYear}`}
                aria-pressed={isSelected}
              >
                {name}
              </Button>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// Sub-component: CustomRangePicker popover
// ---------------------------------------------------------------------------

interface CustomRangePickerProps {
  fromDate: string;
  toDate: string;
  onSelectRange: (from: string, to: string) => void;
}

function CustomRangePicker({
  fromDate,
  toDate,
  onSelectRange,
}: CustomRangePickerProps) {
  const [open, setOpen] = React.useState(false);

  // Seed from the current period as local Dates. Resync when the period changes.
  const [range, setRange] = React.useState<DateRange>({
    from: parseLocalDate(fromDate),
    to: parseLocalDate(toDate),
  });

  React.useEffect(() => {
    if (!open) {
      setRange({
        from: parseLocalDate(fromDate),
        to: parseLocalDate(toDate),
      });
    }
  }, [fromDate, toDate, open]);

  const handleSelect = (selected: DateRange | undefined) => {
    if (!selected) return;
    setRange(selected);
    if (selected.from && selected.to) {
      onSelectRange(
        toLocalDateString(selected.from),
        toLocalDateString(selected.to)
      );
      setOpen(false);
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="gap-1 font-medium"
          aria-label="Open custom date range picker"
        >
          <Calendar className="size-3.5 text-muted-foreground" />
          <span>Custom</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <CalendarPrimitive
          mode="range"
          selected={range}
          onSelect={handleSelect}
          defaultMonth={parseLocalDate(fromDate)}
          numberOfMonths={2}
          disabled={{ after: new Date() }}
        />
      </PopoverContent>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// Sub-component: Presets dropdown
// ---------------------------------------------------------------------------

function PresetsDropdown({ period }: { period: AnalyticsPeriod }) {
  const applyThisMonth = () => {
    const now = new Date();
    period.setMonth(now.getFullYear(), now.getMonth());
  };

  const applyLastMonth = () => {
    const now = new Date();
    let year = now.getFullYear();
    let monthIndex = now.getMonth() - 1;
    if (monthIndex < 0) {
      monthIndex = 11;
      year -= 1;
    }
    period.setMonth(year, monthIndex);
  };

  const applyLast3Months = () => {
    const now = new Date();
    // From the 1st of the month two months ago to today.
    let fromYear = now.getFullYear();
    let fromMonth = now.getMonth() - 2;
    if (fromMonth < 0) {
      fromMonth += 12;
      fromYear -= 1;
    }
    const fromDate = `${String(fromYear).padStart(4, "0")}-${String(fromMonth + 1).padStart(2, "0")}-01`;
    const toDate = toLocalDateString(now);
    period.setRange(fromDate, toDate);
  };

  const applyYearToDate = () => {
    const now = new Date();
    const fromDate = `${now.getFullYear()}-01-01`;
    const toDate = toLocalDateString(now);
    period.setRange(fromDate, toDate);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="gap-1 font-medium"
          aria-label="Open period presets"
        >
          <span>Presets</span>
          <ChevronDown className="size-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-44">
        <DropdownMenuLabel className="text-xs text-muted-foreground">
          Quick select
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={applyThisMonth}>
          This month
        </DropdownMenuItem>
        <DropdownMenuItem onClick={applyLastMonth}>
          Last month
        </DropdownMenuItem>
        <DropdownMenuItem onClick={applyLast3Months}>
          Last 3 months
        </DropdownMenuItem>
        <DropdownMenuItem onClick={applyYearToDate}>
          Year to date
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// ---------------------------------------------------------------------------
// Main export: PeriodSelector
// ---------------------------------------------------------------------------

export function PeriodSelector({ period }: { period: AnalyticsPeriod }) {
  const { fromDate, toDate, label, mode, isActiveMonthDefault } = period;

  const { year: fromYear, monthIndex: fromMonth } =
    yearMonthFromDate(fromDate);

  // ---- Prev month ----
  const handlePrevMonth = () => {
    let year = fromYear;
    let monthIndex = fromMonth - 1;
    if (monthIndex < 0) {
      monthIndex = 11;
      year -= 1;
    }
    period.setMonth(year, monthIndex);
  };

  // ---- Next month ----
  const handleNextMonth = () => {
    let year = fromYear;
    let monthIndex = fromMonth + 1;
    if (monthIndex > 11) {
      monthIndex = 0;
      year += 1;
    }
    period.setMonth(year, monthIndex);
  };

  // Disable "next" if the next month would be in the future.
  const isNextMonthDisabled = (() => {
    const now = new Date();
    const currentYear = now.getFullYear();
    const currentMonth = now.getMonth();
    const nextMonth = fromMonth + 1 > 11 ? 0 : fromMonth + 1;
    const nextYear = fromMonth + 1 > 11 ? fromYear + 1 : fromYear;
    return (
      nextYear > currentYear ||
      (nextYear === currentYear && nextMonth > currentMonth)
    );
  })();

  // ---- Range handler (for CustomRangePicker) ----
  const handleRangeSelect = (from: string, to: string) => {
    period.setRange(from, to);
  };

  // ---- Month handler (for MonthYearPicker) ----
  const handleMonthSelect = (year: number, monthIndex: number) => {
    period.setMonth(year, monthIndex);
  };

  return (
    <div className="flex flex-wrap items-center gap-1">
      {/* Current period label */}
      <div className="mr-2 flex flex-col">
        <span className="text-sm font-semibold leading-tight tracking-tight">
          {label}
        </span>
        {isActiveMonthDefault && (
          <span className="text-xs text-muted-foreground">
            latest activity
          </span>
        )}
        {!isActiveMonthDefault && mode === "range" && (
          <span className="text-xs text-muted-foreground">Custom range</span>
        )}
      </div>

      {/* Prev / Next chevrons */}
      <Button
        variant="outline"
        size="icon-sm"
        onClick={handlePrevMonth}
        aria-label="Previous month"
        className="shrink-0"
      >
        <ChevronLeft className="size-4" />
      </Button>
      <Button
        variant="outline"
        size="icon-sm"
        onClick={handleNextMonth}
        disabled={isNextMonthDisabled}
        aria-label="Next month"
        className="shrink-0"
      >
        <ChevronRight className="size-4" />
      </Button>

      {/* Month-year picker */}
      <MonthYearPicker
        fromDate={fromDate}
        isMonthMode={mode === "month"}
        onSelectMonth={handleMonthSelect}
      />

      {/* Custom range picker */}
      <CustomRangePicker
        fromDate={fromDate}
        toDate={toDate}
        onSelectRange={handleRangeSelect}
      />

      {/* Presets */}
      <PresetsDropdown period={period} />
    </div>
  );
}
