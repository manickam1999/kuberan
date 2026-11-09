"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  CalendarDays,
  Check,
  Paperclip,
  Search,
  Tag,
  Users,
  Wallet,
  X,
} from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ToggleChip } from "@/components/ui/toggle-chip";
import { CategoryPicker } from "@/components/transactions/category-picker";
import { PERIOD_LABELS, type PeriodKey, type TxQueryState } from "@/lib/transaction-list";
import { transactionVisual } from "@/lib/domain-visuals";
import type { Account, Category, TransactionType } from "@/types/models";

const PERIOD_ORDER: PeriodKey[] = [
  "all",
  "this-month",
  "last-month",
  "last-90",
  "custom",
];

const TYPE_ORDER: TransactionType[] = [
  "expense",
  "income",
  "transfer",
  "reimbursement",
  "opening_balance",
  "investment",
];

function OptionRow({
  selected,
  onClick,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="hover:bg-accent focus-visible:bg-accent flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none"
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {selected && <Check className="size-4 shrink-0" aria-hidden />}
    </button>
  );
}

/** A filter chip that opens a popover; filled and solid while a value is set. */
function ChipPopover({
  active,
  icon,
  label,
  children,
  onClear,
  open,
  onOpenChange,
  width = "w-56",
}: {
  active: boolean;
  icon?: ReactNode;
  label: string;
  children: ReactNode;
  onClear?: () => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  width?: string;
}) {
  return (
    <span className="inline-flex shrink-0 items-center">
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>
          <ToggleChip pressed={active} className={cn(active && onClear && "rounded-r-none")}>
            {icon}
            <span className="max-w-[10rem] truncate">{label}</span>
          </ToggleChip>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          // A long account list must scroll inside the popover, not run off screen.
          className={cn(
            "max-h-[min(24rem,var(--radix-popover-content-available-height))] overflow-y-auto p-1.5",
            width
          )}
        >
          {children}
        </PopoverContent>
      </Popover>
      {active && onClear && (
        <ToggleChip
          pressed
          aria-label={`Clear ${label} filter`}
          onClick={onClear}
          className="rounded-l-none border-l-0 px-2"
        >
          <X />
        </ToggleChip>
      )}
    </span>
  );
}

interface FilterBarProps {
  state: TxQueryState;
  onChange: (patch: Partial<TxQueryState>) => void;
  accounts: Account[];
  categories: Category[];
  /** Ref for the keyboard shortcut that focuses the search. */
  searchRef?: React.RefObject<HTMLInputElement | null>;
}

/** Search box plus the filter chips; everything lives in the URL via `onChange`. */
export function TransactionsFilterBar({
  state,
  onChange,
  accounts,
  categories,
  searchRef,
}: FilterBarProps) {
  const [search, setSearch] = useState(state.q);
  const [openChip, setOpenChip] = useState<string | null>(null);
  const [amountDraft, setAmountDraft] = useState({ min: state.min, max: state.max });
  const localRef = useRef<HTMLInputElement>(null);
  const inputRef = searchRef ?? localRef;

  // Follow the URL (back button, "Clear all") without fighting the user's typing.
  const lastPushed = useRef(state.q);
  useEffect(() => {
    if (state.q !== lastPushed.current) {
      lastPushed.current = state.q;
      setSearch(state.q);
    }
  }, [state.q]);

  // Debounce the search into the URL.
  useEffect(() => {
    if (search === lastPushed.current) return;
    const t = setTimeout(() => {
      lastPushed.current = search;
      onChange({ q: search, page: 1 });
    }, 300);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- onChange identity changes with the URL; only the typed value should restart the timer
  }, [search]);

  const toggleOpen = (name: string) => (open: boolean) =>
    setOpenChip(open ? name : null);

  const sortedAccounts = [...accounts].sort((a, b) => a.name.localeCompare(b.name));
  const account = accounts.find((a) => a.id === state.account);
  const category = categories.find((c) => c.id === state.category);
  const amountActive = !!(state.min || state.max);
  const amountLabel = amountActive
    ? `${state.min || "0"} – ${state.max || "any"}`
    : "Amount";

  const periodLabel =
    state.period === "custom" && (state.from || state.to)
      ? `${state.from || "…"} → ${state.to || "…"}`
      : PERIOD_LABELS[state.period];

  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-center">
      <div className="relative md:w-80 md:shrink-0">
        <Search
          className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
          aria-hidden
        />
        <Input
          ref={inputRef}
          type="search"
          inputMode="search"
          aria-label="Search transactions"
          placeholder="Search descriptions, notes, amounts"
          maxLength={100}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pr-9 pl-9 [&::-webkit-search-cancel-button]:hidden"
        />
        {search && (
          <button
            type="button"
            aria-label="Clear search"
            onClick={() => setSearch("")}
            className="text-muted-foreground hover:text-foreground absolute top-1/2 right-2.5 -translate-y-1/2 rounded-sm p-0.5"
          >
            <X className="size-4" />
          </button>
        )}
      </div>

      <div className="[scrollbar-width:none] [&::-webkit-scrollbar]:hidden -mx-4 flex items-center gap-2 overflow-x-auto px-4 pb-1 md:mx-0 md:flex-wrap md:overflow-visible md:px-0 md:pb-0">
        <ChipPopover
          active={state.period !== "all"}
          icon={<CalendarDays />}
          label={periodLabel}
          open={openChip === "period"}
          onOpenChange={toggleOpen("period")}
          onClear={() => onChange({ period: "all", from: "", to: "", page: 1 })}
        >
          {PERIOD_ORDER.map((p) => (
            <OptionRow
              key={p}
              selected={state.period === p}
              onClick={() => {
                onChange({ period: p, page: 1 });
                if (p !== "custom") setOpenChip(null);
              }}
            >
              {PERIOD_LABELS[p]}
            </OptionRow>
          ))}
          {state.period === "custom" && (
            <div className="grid gap-2 p-2">
              <div className="grid gap-1">
                <Label htmlFor="tx-filter-from" className="text-xs">
                  From
                </Label>
                <Input
                  id="tx-filter-from"
                  type="date"
                  value={state.from}
                  onChange={(e) => onChange({ from: e.target.value, page: 1 })}
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="tx-filter-to" className="text-xs">
                  To
                </Label>
                <Input
                  id="tx-filter-to"
                  type="date"
                  value={state.to}
                  onChange={(e) => onChange({ to: e.target.value, page: 1 })}
                />
              </div>
            </div>
          )}
        </ChipPopover>

        <ChipPopover
          active={!!state.account}
          icon={<Wallet />}
          label={account?.name ?? "Account"}
          open={openChip === "account"}
          onOpenChange={toggleOpen("account")}
          onClear={() => onChange({ account: "", page: 1 })}
        >
          <OptionRow
            selected={!state.account}
            onClick={() => {
              onChange({ account: "", page: 1 });
              setOpenChip(null);
            }}
          >
            All accounts
          </OptionRow>
          {sortedAccounts.map((a) => (
            <OptionRow
              key={a.id}
              selected={state.account === a.id}
              onClick={() => {
                onChange({ account: a.id, page: 1 });
                setOpenChip(null);
              }}
            >
              {a.name}{" "}
              <span className="text-muted-foreground text-xs">{a.currency}</span>
            </OptionRow>
          ))}
        </ChipPopover>

        <span className="inline-flex shrink-0 items-center">
          <CategoryPicker
            categories={categories}
            value={state.category}
            allowNone
            noneLabel="Any category"
            onSelect={(id) => onChange({ category: id ?? "", page: 1 })}
          >
            <ToggleChip
              pressed={!!state.category}
              className={cn(state.category && "rounded-r-none")}
            >
              <Tag />
              <span className="max-w-[10rem] truncate">
                {category?.name ?? "Category"}
              </span>
            </ToggleChip>
          </CategoryPicker>
          {state.category && (
            <ToggleChip
              pressed
              aria-label="Clear Category filter"
              onClick={() => onChange({ category: "", page: 1 })}
              className="rounded-l-none border-l-0 px-2"
            >
              <X />
            </ToggleChip>
          )}
        </span>

        <ChipPopover
          active={!!state.type}
          label={state.type ? transactionVisual(state.type).label : "Type"}
          open={openChip === "type"}
          onOpenChange={toggleOpen("type")}
          onClear={() => onChange({ type: "", page: 1 })}
        >
          <OptionRow
            selected={!state.type}
            onClick={() => {
              onChange({ type: "", page: 1 });
              setOpenChip(null);
            }}
          >
            All types
          </OptionRow>
          {TYPE_ORDER.map((t) => (
            <OptionRow
              key={t}
              selected={state.type === t}
              onClick={() => {
                onChange({ type: t, page: 1 });
                setOpenChip(null);
              }}
            >
              {transactionVisual(t).label}
            </OptionRow>
          ))}
        </ChipPopover>

        <ChipPopover
          active={amountActive}
          label={amountLabel}
          open={openChip === "amount"}
          onOpenChange={(open) => {
            if (open) setAmountDraft({ min: state.min, max: state.max });
            toggleOpen("amount")(open);
          }}
          onClear={() => onChange({ min: "", max: "", page: 1 })}
          width="w-64"
        >
          <form
            className="grid gap-3 p-2"
            onSubmit={(e) => {
              e.preventDefault();
              onChange({
                min: amountDraft.min,
                max: amountDraft.max,
                page: 1,
              });
              setOpenChip(null);
            }}
          >
            <p className="text-muted-foreground text-xs">
              Compared with each transaction&apos;s own amount, whatever its currency.
            </p>
            <div className="grid grid-cols-2 gap-2">
              <div className="grid gap-1">
                <Label htmlFor="tx-filter-min" className="text-xs">
                  At least
                </Label>
                <Input
                  id="tx-filter-min"
                  inputMode="decimal"
                  placeholder="0.00"
                  value={amountDraft.min}
                  onChange={(e) =>
                    setAmountDraft((d) => ({
                      ...d,
                      min: e.target.value.replace(/[^\d.]/g, ""),
                    }))
                  }
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="tx-filter-max" className="text-xs">
                  At most
                </Label>
                <Input
                  id="tx-filter-max"
                  inputMode="decimal"
                  placeholder="No limit"
                  value={amountDraft.max}
                  onChange={(e) =>
                    setAmountDraft((d) => ({
                      ...d,
                      max: e.target.value.replace(/[^\d.]/g, ""),
                    }))
                  }
                />
              </div>
            </div>
            <Button type="submit" size="sm">
              Apply
            </Button>
          </form>
        </ChipPopover>

        <ToggleChip
          pressed={state.shared}
          onPressedChange={(v) => onChange({ shared: v, page: 1 })}
        >
          <Users />
          Shared
        </ToggleChip>
        <ToggleChip
          pressed={state.receipt}
          onPressedChange={(v) => onChange({ receipt: v, page: 1 })}
        >
          <Paperclip />
          Has receipt
        </ToggleChip>
      </div>
    </div>
  );
}
