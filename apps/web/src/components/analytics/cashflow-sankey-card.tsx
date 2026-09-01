"use client";

import { useMemo } from "react";
import {
  ThemedSankey,
  type SankeyLinkInput,
  type SankeyNodeInput,
} from "@/components/charts/themed-sankey";
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
import { formatCurrency } from "@/lib/format";
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
): { nodes: SankeyNodeInput[]; amounts: number[] } {
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

  return { nodes, amounts };
}

export function CashflowSankeyCard() {
  const {
    fromDate,
    toDate,
    label: monthLabel,
    isCurrentMonth,
  } = useActiveMonth();
  const { data, isLoading } = useCashflow(fromDate, toDate);
  const { hideBalances } = useAuth();

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

    return { nodes, links, height };
  }, [data]);

  const money = (value: number) =>
    formatCurrency(value, undefined, hideBalances);

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Cash flow</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[360px] w-full" />
        </CardContent>
      </Card>
    );
  }

  const hasFlow =
    data && (data.total_income > 0 || data.total_expenses > 0) && graph;

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="text-base">Cash flow</CardTitle>
            <CardDescription>
              Income sources → spending · {monthLabel}
              {!isCurrentMonth && " · latest activity"}
            </CardDescription>
          </div>
          {data && (data.total_income > 0 || data.total_expenses > 0) && (
            <div className="flex gap-5 text-right">
              <div>
                <p className="text-xs text-muted-foreground">In</p>
                <p className="money text-sm font-semibold text-positive">
                  {money(data.total_income)}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Out</p>
                <p className="money text-sm font-semibold text-negative">
                  {money(data.total_expenses)}
                </p>
              </div>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {hasFlow ? (
          <ThemedSankey
            nodes={graph.nodes}
            links={graph.links}
            valueFormatter={money}
            height={graph.height}
          />
        ) : (
          <div className="py-16 text-center text-sm text-muted-foreground">
            No income or expenses recorded for this period
          </div>
        )}
      </CardContent>
    </Card>
  );
}
