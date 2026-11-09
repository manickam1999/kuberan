"use client";

import { useCallback, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type {
  SankeyLinkInput,
  SankeyNodeInput,
} from "@/components/charts/themed-sankey";
import { LazyThemedSankey as ThemedSankey } from "@/components/charts/lazy";
import { ditherColorAt } from "@/lib/dither";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useCashflow } from "@/hooks/use-transactions";
import { useActiveMonth } from "@/hooks/use-active-month";
import { useAuth } from "@/hooks/use-auth";
import { formatMoney } from "@/lib/format";
import { useIsMobile } from "@/hooks/use-mobile";
import { Money } from "@/components/ui/money";
import { rankedFlow, transactionsHref } from "@/lib/insights";
import { cn } from "@/lib/utils";
import { useBaseCurrency } from "@/hooks/use-profile";
import { FxNotice } from "@/components/ui/fx-notice";
import type { SpendingByCategoryItem } from "@/types/api";

const INCOME_TOP_N = 6;
const EXPENSE_TOP_N = 8;
const HUB_CLEAN = "#8b8b95";
const OTHER_CLEAN = "#8b8b95";
const SAVINGS_CLEAN = "#16a34a";
const DEFICIT_CLEAN = "#dc2626";

const HUB_ID = "hub";
const SAVINGS_ID = "savings";
const DEFICIT_ID = "deficit";

/**
 * Collapses a category breakdown into at most `topN` nodes, folding the tail
 * into a single grey "Other" bucket. `ditherOffset` shifts the palette cycle so
 * the income and expense sides don't reuse identical hues.
 */
function sideNodes(
  items: SpendingByCategoryItem[],
  topN: number,
  prefix: string,
  otherLabel: string,
  ditherOffset: number
): {
  nodes: SankeyNodeInput[];
  amounts: number[];
  refs: Map<string, string | null>;
} {
  const align = prefix === "in" ? "left" : "right";
  const top = items.slice(0, topN);
  const rest = items.slice(topN);

  const nodes: SankeyNodeInput[] = top.map((item, i) => ({
    id: `${prefix}-${i}`,
    label: item.category_name,
    cleanColor: item.category_color || OTHER_CLEAN,
    ditherColor: ditherColorAt(ditherOffset + i),
    align,
  }));
  const amounts = top.map((item) => item.total);
  // Node id -> category id (null: uncategorised); the "Other" bucket is absent.
  const refs = new Map<string, string | null>(
    top.map((item, i) => [`${prefix}-${i}`, item.category_id])
  );

  if (rest.length > 0) {
    nodes.push({
      id: `${prefix}-other`,
      label: otherLabel,
      cleanColor: OTHER_CLEAN,
      ditherColor: "grey",
      align,
    });
    amounts.push(rest.reduce((sum, item) => sum + item.total, 0));
  }

  return { nodes, amounts, refs };
}

interface CashflowSankeyCardProps {
  fromDate?: string;
  toDate?: string;
  label?: string;
  isActiveMonthDefault?: boolean;
}

export function CashflowSankeyCard(props: CashflowSankeyCardProps) {
  const router = useRouter();
  const isMobile = useIsMobile();
  const active = useActiveMonth();
  const fromDate = props.fromDate ?? active.fromDate;
  const toDate = props.toDate ?? active.toDate;
  const monthLabel = props.label ?? active.label;
  const showLatestActivityHint =
    props.fromDate && props.toDate
      ? !!props.isActiveMonthDefault
      : !active.isCurrentMonth;
  const { data, isLoading } = useCashflow(fromDate, toDate);
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  // The response's currency is the base: the unit of every amount here.
  const currency = data?.currency ?? baseCurrency;

  const graph = useMemo(() => {
    if (!data) return null;

    const income = sideNodes(
      data.income.filter((i) => i.total > 0),
      INCOME_TOP_N,
      "in",
      "Other income",
      0
    );
    const expense = sideNodes(
      data.expenses.filter((i) => i.total > 0),
      EXPENSE_TOP_N,
      "ex",
      "Other",
      income.nodes.length
    );

    const nodes: SankeyNodeInput[] = [];
    const links: SankeyLinkInput[] = [];

    income.nodes.forEach((node, i) => {
      nodes.push(node);
      links.push({ source: node.id, target: HUB_ID, value: income.amounts[i] });
    });

    nodes.push({
      id: HUB_ID,
      label: "Cash flow",
      cleanColor: HUB_CLEAN,
      ditherColor: "grey",
      align: "center",
    });

    expense.nodes.forEach((node, i) => {
      nodes.push(node);
      links.push({ source: HUB_ID, target: node.id, value: expense.amounts[i] });
    });

    const surplus = data.total_income - data.total_expenses;
    if (surplus > 0) {
      nodes.push({
        id: SAVINGS_ID,
        label: "Savings",
        cleanColor: SAVINGS_CLEAN,
        ditherColor: "green",
        align: "right",
      });
      links.push({ source: HUB_ID, target: SAVINGS_ID, value: surplus });
    } else if (surplus < 0) {
      // Spending outran income: the shortfall is drawn from prior balance.
      nodes.push({
        id: DEFICIT_ID,
        label: "From savings",
        cleanColor: DEFICIT_CLEAN,
        ditherColor: "red",
        align: "left",
      });
      links.push({ source: DEFICIT_ID, target: HUB_ID, value: -surplus });
    }

    const rows = Math.max(
      income.nodes.length + (surplus < 0 ? 1 : 0),
      expense.nodes.length + (surplus > 0 ? 1 : 0)
    );
    const height = Math.min(620, Math.max(300, rows * 58 + 52));

    const refs = new Map<string, { type: "income" | "expense"; categoryId: string | null }>();
    income.refs.forEach((categoryId, id) => refs.set(id, { type: "income", categoryId }));
    expense.refs.forEach((categoryId, id) => refs.set(id, { type: "expense", categoryId }));

    return { nodes, links, height, refs };
  }, [data]);

  const openNode = useCallback(
    (node: SankeyNodeInput) => {
      const ref = graph?.refs.get(node.id);
      if (ref) {
        router.push(
          transactionsHref({
            from: fromDate,
            to: toDate,
            type: ref.type,
            categoryId: ref.categoryId,
          })
        );
      } else if (node.id === "ex-other") {
        router.push(transactionsHref({ from: fromDate, to: toDate, type: "expense" }));
      } else if (node.id === "in-other") {
        router.push(transactionsHref({ from: fromDate, to: toDate, type: "income" }));
      }
    },
    [graph, router, fromDate, toDate]
  );

  const money = (value: number) => formatMoney(value, currency, hideBalances);

  if (isLoading) {
    return (
      <Card aria-busy="true">
        <CardHeader>
          <CardTitle className="text-base">Income to spending</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[260px] w-full md:h-[360px]" />
        </CardContent>
      </Card>
    );
  }

  const hasFlow =
    data && (data.total_income > 0 || data.total_expenses > 0) && graph;
  const ranked = data && hasFlow ? rankedFlow(data) : [];
  const rankedMax = Math.max(1, ...ranked.map((r) => r.amount));

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="text-base">Income to spending</CardTitle>
            <CardDescription>
              {monthLabel}
              {showLatestActivityHint && " · latest activity"}
            </CardDescription>
            <FxNotice
              fxMissing={data?.fx_missing}
              unconverted={data?.unconverted}
              variant="inline"
              className="mt-1"
            />
          </div>
          {data && (data.total_income > 0 || data.total_expenses > 0) && (
            <div className="flex gap-5 text-right">
              <div>
                <p className="text-xs text-muted-foreground">In</p>
                <p className="money text-sm font-semibold text-positive">
                  <Money amount={data.total_income} currency={currency} sign="never" />
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Out</p>
                <p className="money text-sm font-semibold">
                  <Money amount={data.total_expenses} currency={currency} sign="never" />
                </p>
              </div>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {!hasFlow ? (
          <div className="py-12 text-center text-sm text-muted-foreground">
            No income or expenses recorded for this period.
          </div>
        ) : isMobile ? (
          <ul className="space-y-3">
            {ranked.map((r) => {
              const body = (
                <>
                  <span className="flex items-baseline justify-between gap-3 text-sm">
                    <span className="truncate">{r.label}</span>
                    <Money
                      amount={r.amount}
                      currency={currency}
                      sign="never"
                      className="money shrink-0"
                    />
                  </span>
                  <span className="bg-muted block h-2 overflow-hidden rounded-full">
                    <span
                      className={cn(
                        "block h-full rounded-full",
                        r.kind === "saved" && "bg-positive",
                        r.kind === "deficit" && "bg-negative"
                      )}
                      style={{
                        width: `${(r.amount / rankedMax) * 100}%`,
                        backgroundColor:
                          r.kind === "expense"
                            ? r.color || OTHER_CLEAN
                            : r.kind === "other"
                              ? OTHER_CLEAN
                              : undefined,
                      }}
                    />
                  </span>
                </>
              );
              const href =
                r.kind === "expense"
                  ? transactionsHref({
                      from: fromDate,
                      to: toDate,
                      type: "expense",
                      categoryId: r.categoryId,
                    })
                  : r.kind === "other"
                    ? transactionsHref({ from: fromDate, to: toDate, type: "expense" })
                    : null;
              return (
                <li key={r.key}>
                  {href ? (
                    <Link
                      href={href}
                      className="focus-visible:ring-ring/50 grid gap-1.5 rounded-md outline-none focus-visible:ring-[3px]"
                    >
                      {body}
                    </Link>
                  ) : (
                    <div className="grid gap-1.5">{body}</div>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <ThemedSankey
            nodes={graph.nodes}
            links={graph.links}
            valueFormatter={money}
            height={graph.height}
            onNodeClick={openNode}
          />
        )}
        {hasFlow && (
          <p className="text-xs text-muted-foreground">
            Opening balances and transfers between your own accounts are left
            out.{!isMobile && " Click a band to see its transactions."}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
