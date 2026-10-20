"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, HandCoins, Handshake, Pencil } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { useReceivables, useReceivablesSummary } from "@/hooks/use-receivables";
import { formatCurrency } from "@/lib/format";
import {
  RECEIVABLE_KIND_LABELS,
  RECEIVABLE_KINDS,
} from "@/lib/receivable-format";
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
import type { ReceivableCounterpartySummary, ReceivableFilters } from "@/types/api";
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

export default function ReceivablesPage() {
  const { hideBalances } = useAuth();
  const [status, setStatus] = useState<StatusFilter>("open");
  const [kind, setKind] = useState<ReceivableKind | "all">("all");
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const [lendOpen, setLendOpen] = useState(false);
  const [renaming, setRenaming] = useState<{ key: string; name: string } | null>(
    null
  );

  const { data: summary, isLoading: summaryLoading } = useReceivablesSummary();
  const filters: ReceivableFilters = {
    status: status === "all" ? undefined : status,
    kind: kind === "all" ? undefined : kind,
    page_size: 100,
  };
  const { data: listData, isLoading: listLoading } = useReceivables(filters);

  const money = (cents: number) => formatCurrency(cents, undefined, hideBalances);

  // Group the (filtered) list by counterparty. The summary only covers open
  // items, so non-open views are built from the list endpoint itself.
  const groups = new Map<
    string,
    {
      key: string;
      name: string;
      outstanding: number;
      total: number;
      count: number;
      openCount: number;
    }
  >();
  for (const r of listData?.data ?? []) {
    const g = groups.get(r.counterparty_key) ?? {
      key: r.counterparty_key,
      name: r.counterparty_name,
      outstanding: 0,
      total: 0,
      count: 0,
      openCount: 0,
    };
    g.outstanding += r.outstanding;
    g.total += r.amount;
    g.count += 1;
    if (r.outstanding > 0) g.openCount += 1;
    groups.set(r.counterparty_key, g);
  }
  const counterpartyRows = Array.from(groups.values()).sort(
    (a, b) => b.outstanding - a.outstanding || a.name.localeCompare(b.name)
  );

  const totalOutstanding = summary?.total_outstanding ?? 0;
  const byKind = (summary?.by_kind ?? []).filter((k) => k.outstanding > 0);
  const summaryByKey = new Map<string, ReceivableCounterpartySummary>(
    (summary?.by_counterparty ?? []).map((c) => [c.counterparty_key, c])
  );

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Owed to me</h1>
          <p className="text-sm text-muted-foreground">
            Money others owe you. Shown for information only and not counted in
            your net worth until it is repaid.
          </p>
        </div>
        <Button size="sm" onClick={() => setLendOpen(true)}>
          <Handshake className="size-4" />
          Lend money
        </Button>
      </div>

      {summaryLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full rounded-xl" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="Total outstanding"
            value={money(totalOutstanding)}
            icon={HandCoins}
            tone="primary"
            sub={
              summary
                ? plural(
                    summary.by_counterparty.reduce((s, c) => s + c.open_count, 0),
                    "open item"
                  )
                : undefined
            }
            className="sm:col-span-2 lg:col-span-1"
          />
          {byKind.map((k) => (
            <StatCard
              key={k.kind}
              label={RECEIVABLE_KIND_LABELS[k.kind]}
              value={money(k.outstanding)}
              icon={HandCoins}
              sub={plural(k.open_count, "open item")}
            />
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={status}
          onValueChange={(v) => setStatus(v as StatusFilter)}
        >
          <SelectTrigger className="h-9 w-auto min-w-[8rem]" aria-label="Status">
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
          <SelectTrigger className="h-9 w-auto min-w-[8rem]" aria-label="Kind">
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

      {listLoading ? (
        <Card>
          <CardContent className="space-y-3 py-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : counterpartyRows.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">
            {status === "open" && kind === "all"
              ? "Nobody owes you anything"
              : "Nothing matches these filters"}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Split an expense or lend money and it will show up here.
          </p>
        </div>
      ) : (
        <Card className="overflow-hidden py-0">
          <div className="divide-y divide-border/50">
            {counterpartyRows.map((g) => {
              const expanded = expandedKey === g.key;
              const openCount = summaryByKey.get(g.key)?.open_count ?? g.openCount;
              return (
                <div key={g.key}>
                  <div className="flex items-center gap-1 pr-2">
                    <button
                      type="button"
                      className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left hover:bg-accent/40"
                      onClick={() => setExpandedKey(expanded ? null : g.key)}
                      aria-expanded={expanded}
                    >
                      {expanded ? (
                        <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                      )}
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-medium">
                          {g.outstanding > 0
                            ? `${g.name} owes ${money(g.outstanding)}`
                            : g.name}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {g.outstanding > 0
                            ? `across ${plural(openCount, "item")}`
                            : plural(g.count, "item")}
                        </div>
                      </div>
                    </button>
                    <Button
                      variant="ghost"
                      size="icon"
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
  const { data, isLoading } = useReceivables({
    counterparty: counterpartyKey,
    status,
    kind,
    page_size: 100,
  });
  if (isLoading) return <Skeleton className="my-3 h-14 w-full" />;
  return <ReceivableList receivables={data?.data ?? []} />;
}
