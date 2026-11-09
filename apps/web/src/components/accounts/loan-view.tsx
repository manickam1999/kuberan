"use client";

import { useMemo, useState } from "react";
import { ArrowLeftRight, Pencil } from "lucide-react";
import { AccountActivity } from "@/components/accounts/account-activity";
import { AccountHeader } from "@/components/accounts/account-header";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { ReminderControl } from "@/components/accounts/reminder-control";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { StatStrip, type StatStripItem } from "@/components/ui/stat-strip";
import {
  daysUntil,
  dueDayOf,
  formatDuration,
  loanPayoff,
  monthlyInterestEstimate,
  nextDueByDay,
} from "@/lib/account-math";
import { formatPercentage } from "@/lib/format";
import type { Account } from "@/types/models";

const MONTHS = [
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

function dueText(days: number): string {
  if (days === 0) return "Payment due today";
  if (days === 1) return "Payment due tomorrow";
  return `Payment due in ${days} days`;
}

function monthYear(utcMs: number): string {
  const d = new Date(utcMs);
  return `${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()}`;
}

/** Loan (a `debt` account): amount owed, monthly payment, next due date and payoff estimate. */
export function LoanView({ account }: { account: Account }) {
  const [payOpen, setPayOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const today = useMemo(() => new Date(), []);

  const owed = account.balance;
  const payment = account.monthly_payment ?? 0;
  const dueDay = dueDayOf(account);
  const due = dueDay !== null ? nextDueByDay(dueDay, today) : null;
  const dueDays = due !== null ? daysUntil(due, today) : null;
  const payoff = loanPayoff(owed, account.interest_rate, payment, dueDay, today);
  const monthlyInterest = monthlyInterestEstimate(owed, account.interest_rate);
  // A payment that doesn't cover the interest never clears the loan.
  const stalled = owed > 0 && payment > 0 && payoff === null;

  const stats: StatStripItem[] = [
    {
      label: "Monthly payment",
      value:
        payment > 0 ? <Money amount={payment} currency={account.currency} /> : "Not set",
    },
    {
      label: "Interest rate",
      value: account.interest_rate
        ? `${formatPercentage(account.interest_rate, 2)} a year`
        : "None",
      sub:
        monthlyInterest > 0 ? (
          <>
            about <Money amount={monthlyInterest} currency={account.currency} /> a
            month
          </>
        ) : undefined,
    },
    {
      label: "Due day",
      value: dueDay !== null ? `Day ${dueDay}` : "Not set",
      sub: "of each month",
    },
  ];

  return (
    <div className="space-y-6">
      <AccountHeader
        account={account}
        eyebrow={`Loan${
          account.interest_rate
            ? ` · ${formatPercentage(account.interest_rate, 1)} APR`
            : ""
        } · ${account.currency}`}
        onEdit={() => setEditOpen(true)}
        actions={
          <Button size="sm" onClick={() => setPayOpen(true)}>
            <ArrowLeftRight className="size-4" />
            Make a payment
          </Button>
        }
      />

      <Card className="gap-1 px-5">
        <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          You owe
        </span>
        <Money
          amount={owed}
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

      <StatStrip items={stats} />

      <div className="grid gap-4 md:grid-cols-2">
        <Card className="gap-3 px-5">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Next payment
          </span>
          {due !== null && dueDays !== null ? (
            <div className="flex items-center gap-4">
              <div
                aria-hidden
                className="bg-muted grid size-16 shrink-0 place-items-center rounded-xl text-center leading-none"
              >
                <span>
                  <span className="font-figures block text-2xl">
                    {new Date(due).getUTCDate()}
                  </span>
                  <span className="text-muted-foreground text-xs uppercase">
                    {MONTHS[new Date(due).getUTCMonth()]}
                  </span>
                </span>
              </div>
              <div className="min-w-0">
                <p className="font-medium">
                  {owed > 0 ? dueText(dueDays) : "Nothing to pay"}
                </p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {owed <= 0 ? (
                    "The loan is paid off."
                  ) : payment > 0 ? (
                    <>
                      <Money
                        amount={Math.min(payment, owed + monthlyInterest)}
                        currency={account.currency}
                      />{" "}
                      is the usual payment.
                    </>
                  ) : (
                    "No monthly payment is set."
                  )}
                </p>
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              Set the day of the month it&apos;s due to see the next payment
              here and get a reminder.
            </p>
          )}
          <ReminderControl account={account} />
        </Card>

        <Card className="gap-3 px-5">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Payoff estimate
          </span>
          {owed <= 0 ? (
            <p className="font-medium">Nothing left to pay.</p>
          ) : payoff ? (
            <div className="space-y-1">
              <p className="font-display text-2xl">
                {formatDuration(payoff.months)}
              </p>
              <p className="text-muted-foreground text-sm">
                Paid off around {monthYear(payoff.payoffDate)}
                {payoff.totalInterest > 0 ? (
                  <>
                    , with about{" "}
                    <Money
                      amount={payoff.totalInterest}
                      currency={account.currency}
                    />{" "}
                    in interest along the way.
                  </>
                ) : (
                  ", with no interest."
                )}
              </p>
              <p className="text-muted-foreground text-xs">
                Assumes the rate stays fixed and every payment is on time.
              </p>
            </div>
          ) : stalled ? (
            <div className="space-y-1">
              <p className="font-medium">The balance isn&apos;t falling.</p>
              <p className="text-muted-foreground text-sm">
                The monthly payment is no more than the interest each month
                (about{" "}
                <Money amount={monthlyInterest} currency={account.currency} />
                ). Pay more to start clearing the loan.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-muted-foreground text-sm">
                Set a monthly payment to see when this loan will be paid off.
              </p>
              <Button
                variant="outline"
                size="sm"
                className="w-fit"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="size-4" />
                Set monthly payment
              </Button>
            </div>
          )}
        </Card>
      </div>

      <AccountActivity accountId={account.id} />

      {/* Make a payment: a transfer into this loan from the user's default account. */}
      <CreateTransactionDialog
        open={payOpen}
        onOpenChange={setPayOpen}
        defaultType="transfer"
        defaultToAccountId={account.id}
      />
      <EditAccountDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        account={account}
      />
    </div>
  );
}
