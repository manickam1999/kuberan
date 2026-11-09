"use client";

import { useMemo, useState } from "react";
import { Handshake } from "lucide-react";
import { useAllReceivables, useReceivablesSummary } from "@/hooks/use-receivables";
import { useBaseCurrency } from "@/hooks/use-profile";
import { formatDate } from "@/lib/format";
import { FxNotice } from "@/components/ui/fx-notice";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import { ToggleChip } from "@/components/ui/toggle-chip";
import { LendMoneyDialog } from "@/components/receivables/lend-money-dialog";
import { RenameCounterpartyDialog } from "@/components/receivables/rename-counterparty-dialog";
import { PersonAvatar, PersonCard } from "@/components/receivables/person-card";
import {
  RECEIVABLE_KIND_LABELS,
  RECEIVABLE_KINDS,
  isOpen,
} from "@/lib/receivable-format";
import { groupByPerson, owedStats, type OwedView } from "@/lib/owed";
import type { ReceivableKind } from "@/types/models";

const VIEW_OPTIONS = [
  { value: "open", label: "Open" },
  { value: "settled", label: "Settled" },
  { value: "all", label: "All" },
] as const;

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

function PageSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading">
      <Skeleton className="h-[88px] w-full rounded-card" />
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-52 w-full rounded-card" />
        ))}
      </div>
    </div>
  );
}

export default function OwedPage() {
  const [view, setView] = useState<OwedView>("open");
  const [kind, setKind] = useState<ReceivableKind | null>(null);
  const [lendOpen, setLendOpen] = useState(false);
  const [renaming, setRenaming] = useState<{ key: string; name: string } | null>(
    null
  );

  const baseCurrency = useBaseCurrency();
  const summaryQuery = useReceivablesSummary();
  const listQuery = useAllReceivables({});
  const summary = summaryQuery.data;
  const all = useMemo(() => listQuery.data ?? [], [listQuery.data]);

  const stats = useMemo(() => owedStats(all), [all]);
  const groups = useMemo(() => groupByPerson(all, view, kind), [all, view, kind]);
  const recentlySettled = useMemo(
    () =>
      all
        .filter((r) => !isOpen(r) && r.status === "settled")
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
        .slice(0, 5),
    [all]
  );
  const kindsPresent = useMemo(
    () => RECEIVABLE_KINDS.filter((k) => all.some((r) => r.kind === k)),
    [all]
  );

  const currency = summary?.currency || baseCurrency;
  const loading = listQuery.isLoading || summaryQuery.isLoading;
  const failed = listQuery.isError || summaryQuery.isError;

  const strip: StatStripItem[] = [
    {
      label: "Outstanding",
      value: (
        <Money
          amount={summary?.total_outstanding ?? 0}
          currency={currency}
          sign="never"
        />
      ),
      sub: `${stats.people} ${stats.people === 1 ? "person" : "people"} · ${plural(stats.openItems, "item")}`,
    },
  ];
  if (stats.repaid.length === 1) {
    strip.push({
      label: "Repaid so far",
      value: (
        <Money
          amount={stats.repaid[0].cents}
          currency={stats.repaid[0].currency}
          sign="never"
        />
      ),
      tone: "pos",
      sub: `${plural(stats.settledItems, "item")} settled`,
    });
  }
  if (stats.oldest) {
    strip.push({
      label: "Oldest",
      value: plural(stats.oldest.days, "day"),
      sub: `${stats.oldest.receivable.counterparty_name}${
        stats.oldest.receivable.transaction?.description
          ? ` · ${stats.oldest.receivable.transaction.description}`
          : ""
      }`,
    });
  }

  const nothingAtAll = !loading && !failed && all.length === 0;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-display hidden text-3xl leading-tight md:block">
            Owed to me
          </h1>
          <p className="text-muted-foreground max-w-xl text-sm">
            Money people owe you. It isn&apos;t counted in net worth until
            it&apos;s repaid.
          </p>
        </div>
        <Button size="sm" onClick={() => setLendOpen(true)}>
          <Handshake className="size-4" />
          Lend money
        </Button>
      </div>

      {summary && (
        <FxNotice fxMissing={summary.fx_missing} variant="inline" />
      )}

      {loading ? (
        <PageSkeleton />
      ) : failed ? (
        <div
          role="alert"
          className="flex flex-col items-center rounded-card border border-dashed p-10 text-center"
        >
          <h3 className="text-base font-semibold">Couldn&apos;t load what&apos;s owed</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Check your connection and try again.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            onClick={() => {
              void listQuery.refetch();
              void summaryQuery.refetch();
            }}
          >
            Try again
          </Button>
        </div>
      ) : nothingAtAll ? (
        <div className="flex flex-col items-center justify-center rounded-card border border-dashed p-12 text-center">
          <h3 className="text-lg font-semibold">Nobody owes you anything</h3>
          <p className="text-muted-foreground mt-1 text-sm">
            Lend money or split an expense and it will show up here.
          </p>
          <Button className="mt-4" size="sm" onClick={() => setLendOpen(true)}>
            <Handshake className="size-4" />
            Lend money
          </Button>
        </div>
      ) : (
        <>
          <StatStrip items={strip} />

          <div className="flex flex-wrap items-center gap-2">
            <Segmented
              aria-label="Show"
              value={view}
              onValueChange={setView}
              options={VIEW_OPTIONS}
            />
            {kindsPresent.length > 1 &&
              kindsPresent.map((k) => (
                <ToggleChip
                  key={k}
                  pressed={kind === k}
                  onPressedChange={(on) => setKind(on ? k : null)}
                >
                  {RECEIVABLE_KIND_LABELS[k]}
                </ToggleChip>
              ))}
          </div>

          {groups.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-card border border-dashed p-10 text-center">
              <h3 className="text-base font-semibold">
                {view === "open" && !kind
                  ? "Nobody owes you anything"
                  : "Nothing matches"}
              </h3>
              <p className="text-muted-foreground mt-1 text-sm">
                {view === "open" && !kind
                  ? "Everything has been repaid."
                  : "Try a different view or kind."}
              </p>
              {(view !== "open" || kind) && (
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-4"
                  onClick={() => {
                    setView("open");
                    setKind(null);
                  }}
                >
                  Show open
                </Button>
              )}
            </div>
          ) : (
            <div className="grid items-start gap-4 md:grid-cols-2 lg:grid-cols-3">
              {groups.map((g) => (
                <PersonCard
                  key={g.key}
                  group={g}
                  onRename={() => setRenaming({ key: g.key, name: g.name })}
                />
              ))}
            </div>
          )}

          {view === "open" && recentlySettled.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Recently settled</CardTitle>
              </CardHeader>
              <CardContent className="-my-1 divide-y divide-border/50">
                {recentlySettled.map((r) => (
                  <div key={r.id} className="flex items-center gap-3 py-2.5">
                    <PersonAvatar
                      name={r.counterparty_name}
                      personKey={r.counterparty_key}
                      size="sm"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">
                        {r.counterparty_name}
                        {r.transaction?.description
                          ? ` · ${r.transaction.description}`
                          : ""}
                      </p>
                      <p className="text-muted-foreground text-xs">
                        Settled {formatDate(r.updated_at)}
                      </p>
                    </div>
                    <Money
                      amount={r.paid_amount}
                      currency={r.currency}
                      sign="never"
                      className="money text-sm"
                    />
                    <Badge tone="pos">Settled</Badge>
                  </div>
                ))}
              </CardContent>
            </Card>
          )}
        </>
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
