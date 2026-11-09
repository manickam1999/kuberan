"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Vault } from "lucide-react";
import { useAccounts } from "@/hooks/use-accounts";
import { AccountActivity } from "@/components/accounts/account-activity";
import { AccountHeader } from "@/components/accounts/account-header";
import { CloseFixedDepositDialog } from "@/components/accounts/close-fixed-deposit-dialog";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { fixedDepositProjection } from "@/lib/account-math";
import {
  formatCompounding,
  formatDate,
  formatPayoutFrequency,
  formatPayoutType,
  formatPercentage,
} from "@/lib/format";
import type { Account } from "@/types/models";

const STATUS_LABEL = {
  active: "Active",
  matured: "Matured",
  closed: "Closed",
} as const;

function Term({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-2.5 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right font-medium">{children}</dd>
    </div>
  );
}

/** Fixed deposit: principal, accrued and projected interest, timeline, terms. */
export function FixedDepositView({ account }: { account: Account }) {
  const router = useRouter();
  const [closeOpen, setCloseOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const fd = account.fixed_deposit;
  const rate = account.interest_rate ?? 0;
  const today = useMemo(() => new Date(), []);

  const projection = useMemo(
    () => (fd ? fixedDepositProjection(fd, rate, today) : null),
    [fd, rate, today],
  );
  const linked = fd?.linked_account_id
    ? accountsData?.data.find((a) => a.id === fd.linked_account_id)
    : undefined;

  const status = fd?.status ?? "active";
  const canClose = !!fd && status !== "closed";

  return (
    <div className="space-y-6">
      <AccountHeader
        account={account}
        eyebrow={`Fixed deposit${rate > 0 ? ` · ${formatPercentage(rate, 2)} p.a.` : ""}`}
        onEdit={() => setEditOpen(true)}
        actions={
          canClose ? (
            <Button
              size="sm"
              variant={status === "matured" ? "default" : "outline"}
              onClick={() => setCloseOpen(true)}
            >
              <Vault className="size-4" />
              {status === "matured" ? "Record payout" : "Close early"}
            </Button>
          ) : undefined
        }
      />

      {fd && projection ? (
        <div className="grid gap-4 md:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
          <Card className="gap-4 px-5">
            <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              Principal
            </span>
            <Money
              amount={fd.principal}
              currency={account.currency}
              size="hero"
              layout="stacked"
              dimCents
            />
            {rate > 0 && (
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
                <Badge tone="pos">
                  <Money
                    amount={projection.accrued}
                    currency={account.currency}
                    sign="always"
                  />
                  &nbsp;accrued
                </Badge>
                <span className="text-muted-foreground text-sm">
                  projected{" "}
                  <Money
                    amount={projection.projected}
                    currency={account.currency}
                  />{" "}
                  at maturity
                </span>
              </div>
            )}

            {status !== "closed" && (
              <div className="mt-1 space-y-2">
                <div
                  role="progressbar"
                  aria-label="Term elapsed"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(projection.progress)}
                  className="bg-muted h-2.5 w-full overflow-hidden rounded-full"
                >
                  <div
                    className="bg-info h-full rounded-full"
                    style={{ width: `${Math.max(projection.progress, 1)}%` }}
                  />
                </div>
                <div className="text-muted-foreground grid grid-cols-3 gap-2 text-xs">
                  <span>
                    {formatDate(fd.start_date)}
                    <b className="text-foreground block font-medium">Started</b>
                  </span>
                  <span className="text-center">
                    Today
                    <b className="text-foreground block font-medium">
                      day {projection.elapsedDays} of {projection.totalDays}
                    </b>
                  </span>
                  <span className="text-right">
                    {formatDate(fd.maturity_date)}
                    <b className="text-foreground block font-medium">
                      {status === "matured" ? "Matured" : "Matures"}
                    </b>
                  </span>
                </div>
              </div>
            )}
          </Card>

          <Card className="gap-1 px-5">
            <h2 className="text-sm font-semibold">Terms</h2>
            <dl className="divide-y">
              <Term label="Status">
                <Badge
                  tone={
                    status === "matured"
                      ? "accent"
                      : status === "closed"
                        ? "neutral"
                        : "pos"
                  }
                >
                  {STATUS_LABEL[status]}
                </Badge>
              </Term>
              <Term label="Interest rate">
                {formatPercentage(rate, 2)} p.a.
              </Term>
              <Term label="Payout">
                {formatPayoutType(fd.payout_type)}
                {fd.payout_frequency
                  ? ` · ${formatPayoutFrequency(fd.payout_frequency)}`
                  : ""}
              </Term>
              <Term label="Compounding">
                {formatCompounding(fd.compounding)}
              </Term>
              <Term label="Value at maturity">
                <Money
                  amount={projection.maturityValue}
                  currency={account.currency}
                />
              </Term>
              {linked && <Term label="Pays into">{linked.name}</Term>}
            </dl>
          </Card>
        </div>
      ) : (
        <Card className="px-5">
          <p className="text-muted-foreground text-sm">
            The deposit terms aren&apos;t available for this account.
          </p>
        </Card>
      )}

      <AccountActivity accountId={account.id} />

      <EditAccountDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        account={account}
      />
      {fd && (
        <CloseFixedDepositDialog
          account={account}
          open={closeOpen}
          onOpenChange={setCloseOpen}
          onClosed={() => router.push("/accounts")}
        />
      )}
    </div>
  );
}
