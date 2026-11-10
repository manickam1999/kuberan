"use client";

import { memo, useRef } from "react";
import { toast } from "sonner";
import { ChevronDown, Paperclip } from "lucide-react";

import { cn } from "@/lib/utils";
import { ApiClientError } from "@/lib/api-client";
import { formatDate } from "@/lib/format";
import { transactionVisual } from "@/lib/domain-visuals";
import { dayNetByCurrency, type DayGroup } from "@/lib/transaction-list";
import { useUpdateTransaction } from "@/hooks/use-transactions";
import { Checkbox } from "@/components/ui/checkbox";
import { Money } from "@/components/ui/money";
import { Pill } from "@/components/ui/pill";
import { Tile } from "@/components/ui/tile";
import { TransactionAmount } from "@/components/transactions/transaction-amount";
import {
  CategoryPicker,
  CategoryTile,
} from "@/components/transactions/category-picker";
import {
  SharedKindBadge,
  YourShareText,
} from "@/components/receivables/shared-expense-indicators";
import type { Category, Transaction } from "@/types/models";

/** Types whose category the user can set. */
function isCategorisable(tx: Transaction): boolean {
  return tx.type === "expense" || tx.type === "income";
}

function viaLabel(tx: Transaction): string | null {
  if (tx.source !== "mcp") return null;
  return `via ${tx.source_client_name || "an app"}`;
}

function isRelativeDay(key: string): boolean {
  return dayLabel(key) === "Today" || dayLabel(key) === "Yesterday";
}

function dayLabel(key: string): string {
  const today = new Date();
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  if (key === iso(today)) return "Today";
  const y = new Date(today);
  y.setDate(y.getDate() - 1);
  if (key === iso(y)) return "Yesterday";
  const [yy, mm, dd] = key.split("-").map(Number);
  return new Date(yy, mm - 1, dd).toLocaleDateString("en-US", {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: yy === today.getFullYear() ? undefined : "numeric",
  });
}

export interface TransactionsListProps {
  groups: DayGroup<Transaction>[];
  categories: Category[];
  accountName: (id: string) => string;
  currencyOf: (tx: Transaction) => string;
  selected: ReadonlySet<string>;
  /** Phones: checkboxes are showing. */
  selectMode: boolean;
  onEnterSelectMode: (id: string) => void;
  activeId: string | null;
  onToggle: (id: string) => void;
  onToggleAll: (ids: string[], on: boolean) => void;
  onOpen: (tx: Transaction) => void;
  /** Transaction whose inline category picker is open (the `c` key). */
  pickerId: string | null;
  onPickerChange: (id: string | null) => void;
}

function DayTotal({
  rows,
  currencyOf,
}: {
  rows: Transaction[];
  currencyOf: (tx: Transaction) => string;
}) {
  const nets = dayNetByCurrency(rows, currencyOf);
  if (nets.length === 0) return null;
  return (
    <span
      title="Net cash flow for the day (includes repayments)"
      className="font-figures inline-flex gap-3 text-[13px]"
    >
      {nets.map(([ccy, net]) => (
        <Money
          key={ccy}
          amount={net}
          currency={ccy}
          sign="always"
          className={net > 0 ? "text-positive" : "text-muted-foreground"}
        />
      ))}
    </span>
  );
}

/** Category cell of a desktop row: a pill that opens the picker in place. */
function InlineCategory({
  tx,
  categories,
  open,
  onOpenChange,
}: {
  tx: Transaction;
  categories: Category[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const update = useUpdateTransaction(tx.id);
  if (!isCategorisable(tx)) {
    return <span className="text-muted-foreground text-xs">–</span>;
  }
  const options = categories.filter((c) => c.type === tx.type);
  const label = tx.category?.name ?? "Uncategorised";
  return (
    <CategoryPicker
      categories={options}
      value={tx.category_id}
      open={open}
      onOpenChange={onOpenChange}
      onSelect={(id) =>
        update.mutate(
          { category_id: id },
          {
            onError: (err) =>
              toast.error(
                err instanceof ApiClientError
                  ? err.message
                  : "Couldn't change the category"
              ),
          }
        )
      }
    >
      <button
        type="button"
        aria-label={`Category: ${label}. Change`}
        onClick={(e) => e.stopPropagation()}
        className="focus-visible:ring-ring/50 group -ml-1 inline-flex max-w-full items-center rounded-[7px] outline-none focus-visible:ring-[3px]"
      >
        <Pill
          color={tx.category?.color || undefined}
          asChild={false}
          className={cn(
            "group-hover:border-input group-hover:text-foreground max-w-full",
            !tx.category && "border-dashed"
          )}
        >
          <span className="truncate">{label}</span>
          <ChevronDown className="size-3 shrink-0 opacity-50" aria-hidden />
        </Pill>
      </button>
    </CategoryPicker>
  );
}

const DesktopRow = memo(function DesktopRow(props: {
  tx: Transaction;
  categories: Category[];
  accountName: string;
  selected: boolean;
  active: boolean;
  pickerOpen: boolean;
  onToggle: (id: string) => void;
  onOpen: (tx: Transaction) => void;
  onPickerChange: (id: string | null) => void;
}) {
  const { tx, selected, active } = props;
  const v = transactionVisual(tx.type);
  const via = viaLabel(tx);
  const title = tx.description || v.label;
  return (
    <tr
      data-tx-id={tx.id}
      data-state={selected ? "selected" : undefined}
      onClick={() => props.onOpen(tx)}
      className={cn(
        "hover:bg-accent/40 h-10 cursor-pointer border-b border-border/50 transition-colors last:border-b-0",
        selected && "bg-primary/[0.07]",
        active && "bg-accent/60 ring-ring/60 ring-1 ring-inset"
      )}
    >
      <td className="w-10 pl-4" onClick={(e) => e.stopPropagation()}>
        <Checkbox
          checked={selected}
          onCheckedChange={() => props.onToggle(tx.id)}
          aria-label={`Select ${title}`}
        />
      </td>
      <td className="max-w-0 px-2">
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            props.onOpen(tx);
          }}
          className="focus-visible:ring-ring/50 flex w-full min-w-0 items-center gap-2.5 rounded-sm text-left outline-none focus-visible:ring-[3px]"
        >
          {tx.category ? (
            <CategoryTile category={tx.category} />
          ) : (
            <Tile size="sm" className={v.chip}>
              <v.icon />
            </Tile>
          )}
          <span className="truncate text-[13.5px] font-medium">{title}</span>
          {(tx.attachments_count ?? 0) > 0 && (
            <>
              <Paperclip
                className="text-muted-foreground size-3 shrink-0"
                aria-hidden
              />
              <span className="sr-only">Has receipts</span>
            </>
          )}
        </button>
      </td>
      <td className="w-[11rem] px-2" onClick={(e) => e.stopPropagation()}>
        <InlineCategory
          tx={tx}
          categories={props.categories}
          open={props.pickerOpen}
          onOpenChange={(o) => props.onPickerChange(o ? tx.id : null)}
        />
      </td>
      <td className="text-muted-foreground w-[10rem] max-w-[10rem] truncate px-2 text-[13px]">
        {props.accountName}
      </td>
      <td className="text-muted-foreground hidden max-w-0 px-2 text-[13px] xl:table-cell">
        <span className="flex min-w-0 items-center gap-1.5">
          <SharedKindBadge transaction={tx} />
          <span className="truncate">
            {tx.notes || via || ""}
          </span>
          {tx.notes && via && (
            <span className="shrink-0 text-xs opacity-70">{via}</span>
          )}
        </span>
      </td>
      <td className="w-[9.5rem] pr-4 pl-2 text-right">
        <span className={cn("font-figures text-[14px]", v.text)}>
          <TransactionAmount transaction={tx} direction={v.sign} />
        </span>
        <YourShareText
          transaction={tx}
          className="text-muted-foreground block text-[11px] leading-tight"
        />
      </td>
    </tr>
  );
});

function MobileRow(props: {
  tx: Transaction;
  accountName: string;
  selected: boolean;
  active: boolean;
  selectMode: boolean;
  onToggle: (id: string) => void;
  onOpen: (tx: Transaction) => void;
  onEnterSelectMode: (id: string) => void;
}) {
  const { tx, selected, selectMode } = props;
  const v = transactionVisual(tx.type);
  const via = viaLabel(tx);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressed = useRef(false);

  const cancel = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  };

  return (
    <div
      data-tx-id={tx.id}
      className={cn(
        "flex items-center gap-3 border-b border-border/50 px-4 last:border-b-0",
        selected && "bg-primary/[0.07]"
      )}
    >
      {selectMode && (
        <Checkbox
          checked={selected}
          onCheckedChange={() => props.onToggle(tx.id)}
          aria-label={`Select ${tx.description || v.label}`}
          className="size-5"
        />
      )}
      <button
        type="button"
        onClick={() => {
          if (longPressed.current) {
            longPressed.current = false;
            return;
          }
          if (selectMode) props.onToggle(tx.id);
          else props.onOpen(tx);
        }}
        onPointerDown={(e) => {
          if (selectMode || e.pointerType === "mouse") return;
          longPressed.current = false;
          timer.current = setTimeout(() => {
            longPressed.current = true;
            props.onEnterSelectMode(tx.id);
          }, 450);
        }}
        onPointerUp={cancel}
        onPointerLeave={cancel}
        onPointerCancel={cancel}
        onContextMenu={(e) => {
          if (timer.current || longPressed.current) e.preventDefault();
        }}
        className="focus-visible:ring-ring/50 flex min-h-[60px] min-w-0 flex-1 items-center gap-3 py-2.5 text-left outline-none select-none focus-visible:ring-[3px]"
      >
        {tx.category ? (
          <CategoryTile category={tx.category} size="md" />
        ) : (
          <Tile size="md" className={v.chip}>
            <v.icon />
          </Tile>
        )}
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1.5">
            <span className="truncate text-sm font-medium">
              {tx.description || v.label}
            </span>
            {(tx.attachments_count ?? 0) > 0 && (
              <>
                <Paperclip
                  className="text-muted-foreground size-3 shrink-0"
                  aria-hidden
                />
                <span className="sr-only">Has receipts</span>
              </>
            )}
          </span>
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
            <SharedKindBadge transaction={tx} />
            <span className="truncate">
              {[props.accountName, tx.category?.name, via]
                .filter(Boolean)
                .join(" · ")}
            </span>
          </span>
        </span>
        <span className="shrink-0 text-right">
          <span className={cn("font-figures block text-sm", v.text)}>
            <TransactionAmount transaction={tx} direction={v.sign} />
          </span>
          <YourShareText transaction={tx} />
        </span>
      </button>
    </div>
  );
}

/**
 * The Transactions list: a 40px-row table on desktop with sticky day totals
 * and an inline category picker, a grouped list on phones.
 */
export function TransactionsList(props: TransactionsListProps) {
  const { groups, selected } = props;
  const allIds = groups.flatMap((g) => g.items.map((t) => t.id));
  const selectedCount = allIds.filter((id) => selected.has(id)).length;
  const allChecked = allIds.length > 0 && selectedCount === allIds.length;
  const someChecked = selectedCount > 0 && !allChecked;

  return (
    <>
      {/* Desktop */}
      <div className="bg-card hidden overflow-clip rounded-card border md:block">
        <table className="w-full table-fixed border-collapse">
          <caption className="sr-only">Transactions grouped by day</caption>
          <thead>
            <tr className="text-muted-foreground h-9 border-b text-left text-xs font-medium">
              <th className="w-10 pl-4" scope="col">
                <Checkbox
                  checked={allChecked ? true : someChecked ? "indeterminate" : false}
                  onCheckedChange={(c) => props.onToggleAll(allIds, c === true)}
                  aria-label="Select all on this page"
                />
              </th>
              <th className="px-2 font-medium" scope="col">
                Description
              </th>
              <th className="w-[11rem] px-2 font-medium" scope="col">
                Category
              </th>
              <th className="w-[10rem] px-2 font-medium" scope="col">
                Account
              </th>
              <th className="hidden px-2 font-medium xl:table-cell" scope="col">
                Notes
              </th>
              <th className="w-[9.5rem] pr-4 pl-2 text-right font-medium" scope="col">
                Amount
              </th>
            </tr>
          </thead>
          {groups.map((g) => (
            <tbody key={g.key}>
              <tr>
                <th
                  scope="rowgroup"
                  colSpan={3}
                  className="bg-muted sticky top-0 z-10 h-8 border-b px-4 text-left text-xs font-medium"
                >
                  {dayLabel(g.key)}
                  {isRelativeDay(g.key) && (
                    <span className="text-muted-foreground ml-2 font-normal">
                      {formatDate(g.key)}
                    </span>
                  )}
                </th>
                <td
                  colSpan={3}
                  className="bg-muted sticky top-0 z-10 h-8 border-b pr-4 text-right"
                >
                  <DayTotal rows={g.items} currencyOf={props.currencyOf} />
                </td>
              </tr>
              {g.items.map((tx) => (
                <DesktopRow
                  key={tx.id}
                  tx={tx}
                  categories={props.categories}
                  accountName={props.accountName(tx.account_id)}
                  selected={selected.has(tx.id)}
                  active={props.activeId === tx.id}
                  pickerOpen={props.pickerId === tx.id}
                  onToggle={props.onToggle}
                  onOpen={props.onOpen}
                  onPickerChange={props.onPickerChange}
                />
              ))}
            </tbody>
          ))}
        </table>
      </div>

      {/* Phones */}
      <div className="bg-card overflow-clip rounded-card border md:hidden">
        {groups.map((g) => (
          <section key={g.key} aria-label={dayLabel(g.key)}>
            <div className="bg-muted sticky top-0 z-10 flex h-8 items-center justify-between border-b px-4">
              <span className="text-xs font-medium">{dayLabel(g.key)}</span>
              <DayTotal rows={g.items} currencyOf={props.currencyOf} />
            </div>
            {g.items.map((tx) => (
              <MobileRow
                key={tx.id}
                tx={tx}
                accountName={props.accountName(tx.account_id)}
                selected={selected.has(tx.id)}
                active={props.activeId === tx.id}
                selectMode={props.selectMode}
                onToggle={props.onToggle}
                onOpen={props.onOpen}
                onEnterSelectMode={props.onEnterSelectMode}
              />
            ))}
          </section>
        ))}
      </div>
    </>
  );
}

/** Skeleton shaped like the list (desktop rows and phone rows). */
export function TransactionsListSkeleton() {
  return (
    <div
      className="bg-card overflow-clip rounded-card border"
      aria-busy="true"
      aria-label="Loading transactions"
    >
      {Array.from({ length: 9 }).map((_, i) => (
        <div
          key={i}
          className="flex h-[60px] items-center gap-3 border-b border-border/50 px-4 last:border-b-0 md:h-10"
        >
          <div className="bg-muted size-8 animate-pulse rounded-control md:size-6" />
          <div className="flex-1 space-y-1.5">
            <div className="bg-muted h-3.5 w-1/3 animate-pulse rounded" />
            <div className="bg-muted h-3 w-1/4 animate-pulse rounded md:hidden" />
          </div>
          <div className="bg-muted hidden h-5 w-24 animate-pulse rounded-[7px] md:block" />
          <div className="bg-muted h-4 w-20 animate-pulse rounded" />
        </div>
      ))}
    </div>
  );
}
