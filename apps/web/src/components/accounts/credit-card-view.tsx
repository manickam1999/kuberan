"use client";

import { useMemo, useState } from "react";
import { ArrowLeftRight, Plus } from "lucide-react";
import { AccountActivity } from "@/components/accounts/account-activity";
import { AccountHeader } from "@/components/accounts/account-header";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { CreateTransactionDialog } from "@/components/transactions/create-transaction-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import {
  daysUntil,
  monthlyInterestEstimate,
  nextDueDate,
  utilisation,
} from "@/lib/account-math";
import { formatPercentage } from "@/lib/format";
import { cn } from "@/lib/utils";
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

/** Credit card: what you owe, utilisation, next due date and interest. */
export function CreditCardView({ account }: { account: Account }) {
  const [payOpen, setPayOpen] = useState(false);
  const [txOpen, setTxOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const today = useMemo(() => new Date(), []);

  const owed = account.balance;
  const limit = account.credit_limit ?? 0;
  const used = utilisation(owed, limit);
  const available = limit > 0 ? Math.max(limit - owed, 0) : null;
  const due = nextDueDate(account.due_date, today);
  const dueDays = due !== null ? daysUntil(due, today) : null;
  const interest = monthlyInterestEstimate(owed, account.interest_rate);

  const barTone =
    used === null
      ? "bg-primary"
      : used > 75
        ? "bg-negative"
        : used > 50
          ? "bg-warning"
          : "bg-primary";

  return (
    <div className="space-y-6">
      <AccountHeader
        account={account}
        eyebrow={`Credit card${
          account.interest_rate
            ? ` · ${formatPercentage(account.interest_rate, 1)} APR`
            : ""
        }`}
        onEdit={() => setEditOpen(true)}
        actions={
          <>
            <Button size="sm" variant="outline" onClick={() => setTxOpen(true)}>
              <Plus className="size-4" />
              Add transaction
            </Button>
            <Button size="sm" onClick={() => setPayOpen(true)}>
              <ArrowLeftRight className="size-4" />
              Pay card
            </Button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-2">
        <Card className="gap-3 px-5">
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
          {used !== null && available !== null && (
            <div className="space-y-2">
              <div
                role="progressbar"
                aria-label="Credit used"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(Math.min(used, 100))}
                className="bg-muted h-2.5 w-full overflow-hidden rounded-full"
              >
                <div
                  className={cn("h-full rounded-full", barTone)}
                  style={{ width: `${Math.max(Math.min(used, 100), 1)}%` }}
                />
              </div>
              <div className="text-muted-foreground flex flex-wrap justify-between gap-x-4 text-xs">
                <span>{formatPercentage(used, 0)} used</span>
                <span>
                  <Money amount={available} currency={account.currency} />{" "}
                  available of{" "}
                  <Money amount={limit} currency={account.currency} />
                </span>
              </div>
            </div>
          )}
        </Card>

        {due !== null && dueDays !== null && (
          <Card className="gap-3 px-5">
            <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              Next payment
            </span>
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
                {owed > 0 && interest > 0 ? (
                  <p className="text-muted-foreground mt-1 text-sm">
                    Pay the full balance to avoid about{" "}
                    <Money amount={interest} currency={account.currency} /> in
                    interest this cycle.
                  </p>
                ) : (
                  <p className="text-muted-foreground mt-1 text-sm">
                    {owed > 0
                      ? "No interest rate is set on this card."
                      : "The card is paid off."}
                  </p>
                )}
              </div>
            </div>
          </Card>
        )}
      </div>

      <AccountActivity accountId={account.id} />

      {/* Pay card: a transfer into this card from the user's default account. */}
      <CreateTransactionDialog
        open={payOpen}
        onOpenChange={setPayOpen}
        defaultType="transfer"
        defaultToAccountId={account.id}
      />
      <CreateTransactionDialog
        open={txOpen}
        onOpenChange={setTxOpen}
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
