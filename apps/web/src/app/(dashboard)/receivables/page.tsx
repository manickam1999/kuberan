"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, HandCoins, Handshake, Pencil } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useAllReceivables, useReceivablesSummary } from "@/hooks/use-receivables";
import { formatMoney } from "@/lib/format";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";
import {
  RECEIVABLE_KIND_ICONS,
  RECEIVABLE_KIND_LABELS,
  RECEIVABLE_KINDS,
} from "@/lib/receivable-format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { StatCard } from "@/components/ui/stat-card";
import { ReceivableList } from "@/components/receivables/receivable-list";
import { LendMoneyDialog } from "@/components/receivables/lend-money-dialog";
import { RenameCounterpartyDialog } from "@/components/receivables/rename-counterparty-dialog";
import type { ReceivableFilters } from "@/types/api";
import type { ReceivableKind } from "@/types/models";

type StatusFilter = "open" | "all" | "settled" | "written_off";

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "all", label: "All" },
  { value: "settled", label: "Settled" },
  { value: "written_off", label: "Written off" },
];

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

interface CounterpartyGroup {
  key: string;
  name: string;
  /** Outstanding per currency: one base-currency entry in the default view, else one per receivable currency. */
  outstanding: [currency: string, cents: number][];
  hasOutstanding: boolean;
  count: number;
  openCount: number;
  settledCount: number;
  writtenOffCount: number;
}

export default function ReceivablesPage() {
  const { hideBalances } = useAuth();
  const [status, setStatus] = useState<StatusFilter>("open");
  const [kind, setKind] = useState<ReceivableKind | "all">("all");
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const [lendOpen, setLendOpen] = useState(false);
  const [renaming, setRenaming] = useState<{ key: string; name: string } | null>(
    null
  );

  const filters: ReceivableFilters = {
    status: status === "all" ? undefined : status,
    kind: kind === "all" ? undefined : kind,
  };
  const defaultView = status === "open" && kind === "all";

  const { data: summary, isLoading: summaryLoading } = useReceivablesSummary();
  // Other views are built from the complete (all pages) filtered list.
  const { data: allItems, isLoading: listLoading } = useAllReceivables(
    filters,
    !defaultView
  );

  const baseCurrency = useBaseCurrency();
  const summaryCurrency = summary?.currency || baseCurrency;
  // Summary totals are already converted to the base currency.
  const money = (cents: number) =>
    formatMoney(cents, summaryCurrency, hideBalances);

  // Default view (Open + All kinds): the summary is complete and authoritative.
  // Otherwise group the full filtered list, so counts and amounts agree.
  let counterpartyRows: CounterpartyGroup[];
  if (defaultView) {
    counterpartyRows = (summary?.by_counterparty ?? []).map((c) => ({
      key: c.counterparty_key,
      name: c.counterparty_name,
      outstanding: [[summaryCurrency, c.outstanding]],
      hasOutstanding: c.outstanding > 0,
      count: c.open_count,
      openCount: c.open_count,
      settledCount: 0,
      writtenOffCount: 0,
    }));
  } else {
    // Receivables in different currencies are never added together: each
    // counterparty carries one outstanding total per currency.
    const groups = new Map<string, CounterpartyGroup>();
    const perCurrency = new Map<string, Map<string, number>>();
    // The list is ordered newest expense first, so the first name seen per key
    // is the one to show unless a later (older) item was created more recently.
    const latestCreated = new Map<string, string>();
    for (const r of allItems ?? []) {
      const g = groups.get(r.counterparty_key) ?? {
        key: r.counterparty_key,
        name: r.counterparty_name,
        outstanding: [],
        hasOutstanding: false,
        count: 0,
        openCount: 0,
        settledCount: 0,
        writtenOffCount: 0,
      };
      const seen = latestCreated.get(r.counterparty_key);
      if (!seen || r.created_at > seen) {
        latestCreated.set(r.counterparty_key, r.created_at);
        g.name = r.counterparty_name;
      }
      const totals = perCurrency.get(r.counterparty_key) ?? new Map<string, number>();
      totals.set(r.currency, (totals.get(r.currency) ?? 0) + r.outstanding);
      perCurrency.set(r.counterparty_key, totals);
      g.count += 1;
      if (r.outstanding > 0) g.openCount += 1;
      else if (r.status === "written_off") g.writtenOffCount += 1;
      else g.settledCount += 1;
      groups.set(r.counterparty_key, g);
    }
    for (const g of groups.values()) {
      g.outstanding = Array.from(perCurrency.get(g.key) ?? []).filter(
        ([, cents]) => cents > 0
      );
      g.hasOutstanding = g.outstanding.length > 0;
    }
    counterpartyRows = Array.from(groups.values()).sort(
      (a, b) =>
        Number(b.hasOutstanding) - Number(a.hasOutstanding) ||
        a.name.localeCompare(b.name)
    );
  }
  const rowsLoading = defaultView ? summaryLoading : listLoading;

  const totalOutstanding = summary?.total_outstanding ?? 0;
  const byCurrency = (summary?.by_currency ?? []).filter(
    (c) => c.outstanding > 0
  );
  const foreignTotals = byCurrency.filter((c) => c.currency !== summaryCurrency);
  const byKind = (summary?.by_kind ?? []).filter((k) => k.outstanding > 0);

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-3xl leading-tight">Owed to me</h1>
          <p className="text-sm text-muted-foreground">
            Money others owe you. Shown for information only and not counted in
            your net worth until it is repaid.
          </p>
        </div>
        <Button className="h-10 sm:h-9" size="sm" onClick={() => setLendOpen(true)}>
          <Handshake className="size-4" />
          Lend money
        </Button>
      </div>

      {summary && <FxNotice fxMissing={summary.fx_missing} />}

      {summaryLoading ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
          <Skeleton className="col-span-2 h-28 w-full rounded-xl lg:col-span-1" />
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full rounded-xl" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
          <StatCard
            label="Total outstanding"
            value={money(totalOutstanding)}
            icon={HandCoins}
            tone="primary"
            sub={
              summary ? (
                <>
                  {plural(
                    summary.by_counterparty.reduce((s, c) => s + c.open_count, 0),
                    "open item"
                  )}
                  {foreignTotals.length > 0 && (
                    <span className="block">
                      {byCurrency
                        .map((c) =>
                          formatMoney(c.outstanding, c.currency, hideBalances)
                        )
                        .join(" + ")}
                    </span>
                  )}
                </>
              ) : undefined
            }
            className="col-span-2 lg:col-span-1"
          />
          {byKind.map((k, i) => (
            <StatCard
              key={k.kind}
              label={RECEIVABLE_KIND_LABELS[k.kind]}
              value={money(k.outstanding)}
              icon={RECEIVABLE_KIND_ICONS[k.kind]}
              sub={plural(k.open_count, "open item")}
              // An odd last card spans the row so no card is orphaned below lg.
              className={cn(
                // Smaller figure on mobile so 5-digit amounts fit half-width cards.
                "[&_.money]:text-lg sm:[&_.money]:text-[1.65rem]",
                i === byKind.length - 1 &&
                  byKind.length % 2 === 1 &&
                  "col-span-2 lg:col-span-1"
              )}
            />
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={status}
          onValueChange={(v) => setStatus(v as StatusFilter)}
        >
          <SelectTrigger className="min-w-[8rem]" aria-label="Status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={kind}
          onValueChange={(v) => setKind(v as ReceivableKind | "all")}
        >
          <SelectTrigger className="min-w-[8rem]" aria-label="Kind">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All kinds</SelectItem>
            {RECEIVABLE_KINDS.map((k) => (
              <SelectItem key={k} value={k}>
                {RECEIVABLE_KIND_LABELS[k]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {rowsLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : counterpartyRows.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          {defaultView ? (
            <>
              <h3 className="text-lg font-semibold">Nobody owes you anything</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Split an expense or lend money and it will show up here.
              </p>
            </>
          ) : (
            <>
              <h3 className="text-lg font-semibold">Nothing matches these filters</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Try a different status or kind.
              </p>
              <Button
                variant="outline"
                className="mt-4 h-10 sm:h-9"
                onClick={() => {
                  setStatus("open");
                  setKind("all");
                  setExpandedKey(null);
                }}
              >
                Clear filters
              </Button>
            </>
          )}
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          <div className="divide-y divide-border/50">
            {counterpartyRows.map((g) => {
              const expanded = expandedKey === g.key;
              return (
                <div key={g.key}>
                  <div className="flex items-center hover:bg-accent/40">
                    <button
                      type="button"
                      className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left outline-none focus-visible:bg-accent/40"
                      onClick={() => setExpandedKey(expanded ? null : g.key)}
                      aria-expanded={expanded}
                    >
                      {expanded ? (
                        <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                      )}
                      <div className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {g.name}
                        </span>
                        <div className="text-xs text-muted-foreground">
                          {g.hasOutstanding
                            ? plural(g.openCount, "open item")
                            : `${plural(g.count, "item")} · ${
                                g.writtenOffCount === 0
                                  ? "settled"
                                  : g.settledCount === 0
                                    ? "written off"
                                    : "settled or written off"
                              }`}
                        </div>
                      </div>
                      <span
                        className={cn(
                          "money shrink-0 text-right text-sm font-semibold",
                          !g.hasOutstanding && "text-muted-foreground"
                        )}
                      >
                        {g.hasOutstanding
                          ? g.outstanding.map(([ccy, cents]) => (
                              <span key={ccy} className="block">
                                {formatMoney(cents, ccy, hideBalances)}
                              </span>
                            ))
                          : "Nothing owed"}
                      </span>
                    </button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-11 shrink-0 rounded-none"
                      aria-label={`Rename ${g.name}`}
                      onClick={() => setRenaming({ key: g.key, name: g.name })}
                    >
                      <Pencil className="size-4" />
                    </Button>
                  </div>
                  {expanded && (
                    <div className="bg-muted/20 px-4 pb-2">
                      <CounterpartyReceivables
                        counterpartyKey={g.key}
                        status={filters.status}
                        kind={filters.kind}
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </Card>
      )}

      <LendMoneyDialog open={lendOpen} onOpenChange={setLendOpen} />
      <RenameCounterpartyDialog
        counterparty={renaming}
        open={renaming !== null}
        onOpenChange={(o) => {
          if (!o) setRenaming(null);
        }}
      />
    </div>
  );
}

function CounterpartyReceivables({
  counterpartyKey,
  status,
  kind,
}: {
  counterpartyKey: string;
  status: ReceivableFilters["status"];
  kind: ReceivableFilters["kind"];
}) {
  const { data, isLoading } = useAllReceivables({
    counterparty: counterpartyKey,
    status,
    kind,
  });
  if (isLoading) return <Skeleton className="my-3 h-14 w-full" />;
  return <ReceivableList receivables={data ?? []} showCounterparty={false} />;
}
