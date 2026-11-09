"use client";

import { useMemo, useState } from "react";
import { Handshake, Plus } from "lucide-react";
import { useAccountTransactions } from "@/hooks/use-transactions";
import { AccountActivity } from "@/components/accounts/account-activity";
import { BalanceHistoryCard } from "@/components/accounts/balance-history-card";
import { AccountHeader } from "@/components/accounts/account-header";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { LendMoneyDialog } from "@/components/receivables/lend-money-dialog";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import { accountFlows } from "@/lib/account-math";
import { calendarMonthRange } from "@/lib/format";
import { isSettlementAccountType } from "@/lib/receivable-format";
import type { Account } from "@/types/models";

const MONTH_PAGE_SIZE = 100;

/** Cash account: balance hero, this month's money in and out, activity. */
export function CashAccountView({ account }: { account: Account }) {
  const [txOpen, setTxOpen] = useState(false);
  const [lendOpen, setLendOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);

  const month = useMemo(() => {
    const now = new Date();
    return calendarMonthRange(now.getFullYear(), now.getMonth());
  }, []);
  const { data: monthData } = useAccountTransactions(account.id, {
    from_date: month.fromDate,
    to_date: month.toDate,
    page: 1,
    page_size: MONTH_PAGE_SIZE,
  });
  // Totals are only honest when every transaction of the month was loaded.
  const complete = !!monthData && monthData.total_items <= MONTH_PAGE_SIZE;
  const flows = complete ? accountFlows(monthData.data, account.id) : null;

  const stats: StatStripItem[] = [
    {
      label: "In this month",
      value: flows ? (
        <Money amount={flows.in} currency={account.currency} sign="always" />
      ) : (
        "—"
      ),
      tone: flows && flows.in > 0 ? "pos" : "default",
    },
    {
      label: "Out this month",
      value: flows ? (
        <Money amount={-flows.out} currency={account.currency} />
      ) : (
        "—"
      ),
    },
    {
      label: "Transactions",
      value: monthData ? String(monthData.total_items) : "—",
      sub: month.label,
    },
    {
      label: "Pinned",
      value: account.is_pinned ? "On Home" : "Not pinned",
    },
  ];

  return (
    <div className="space-y-6">
      <AccountHeader
        account={account}
        eyebrow={`${account.type === "cash" ? "Cash" : "Account"} · ${account.currency}`}
        onEdit={() => setEditOpen(true)}
        actions={
          <>
            {isSettlementAccountType(account.type) && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setLendOpen(true)}
              >
                <Handshake className="size-4" />
                Lend money
              </Button>
            )}
            <Button size="sm" onClick={() => setTxOpen(true)}>
              <Plus className="size-4" />
              Add transaction
            </Button>
          </>
        }
      />

      <Card className="gap-1 px-5">
        <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          Balance
        </span>
        <Money
          amount={account.balance}
          currency={account.currency}
          baseAmount={account.balance_base}
          fxRate={account.fx_rate}
          fxStale={account.fx_stale}
          fxMissing={account.fx_missing}
          size="hero"
          layout="stacked"
          dimCents
        />
      </Card>

      <BalanceHistoryCard accountId={account.id} currency={account.currency} />

      <StatStrip items={stats} />

      <AccountActivity accountId={account.id} />

      <CreateTransactionDialog
        open={txOpen}
        onOpenChange={setTxOpen}
        defaultAccountId={account.id}
      />
      <LendMoneyDialog
        open={lendOpen}
        onOpenChange={setLendOpen}
        defaultAccountId={account.id}
      />
      <EditAccountDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        account={account}
      />
    </div>
  );
}
