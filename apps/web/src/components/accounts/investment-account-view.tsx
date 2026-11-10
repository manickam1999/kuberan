"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronLeft, ChevronRight, ExternalLink, Plus } from "lucide-react";
import {
  useAccountInvestments,
  usePrefetchInvestment,
} from "@/hooks/use-investments";
import { useAuth } from "@/hooks/use-auth";
import { useBaseCurrency } from "@/hooks/use-profile";
import { AccountHeader } from "@/components/accounts/account-header";
import { EditAccountDialog } from "@/components/accounts/edit-account-dialog";
import { AddInvestmentDialog } from "@/components/investments/add-investment-dialog";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatMoney } from "@/lib/format";
import type { Account } from "@/types/models";

const PAGE_SIZE = 20;

/** Investment account: balance, broker details and its holdings. */
export function InvestmentAccountView({ account }: { account: Account }) {
  const router = useRouter();
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();
  const prefetchInvestment = usePrefetchInvestment();
  const [page, setPage] = useState(1);
  const [addOpen, setAddOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);

  const { data, isLoading, isError, refetch } = useAccountInvestments(
    account.id,
    {
      page,
      page_size: PAGE_SIZE,
    },
  );
  const investments = data?.data ?? [];
  const totalPages = data?.total_pages ?? 1;
  const totalItems = data?.total_items ?? 0;

  return (
    <div className="space-y-6">
      <AccountHeader
        account={account}
        eyebrow={`Investment${account.broker ? ` · ${account.broker}` : ""}`}
        onEdit={() => setEditOpen(true)}
        actions={
          <>
            <Button asChild size="sm" variant="outline">
              <Link href="/investments">
                <ExternalLink className="size-4" />
                Portfolio
              </Link>
            </Button>
            <Button size="sm" onClick={() => setAddOpen(true)}>
              <Plus className="size-4" />
              Add investment
            </Button>
          </>
        }
      />

      <Card className="gap-1 px-5">
        <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          Cash balance
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
        {account.account_number && (
          <span className="text-muted-foreground mt-1 text-sm">
            Account {account.account_number}
          </span>
        )}
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Holdings</CardTitle>
          {totalItems > 0 && (
            <CardDescription>
              {totalItems} holding{totalItems !== 1 ? "s" : ""}
            </CardDescription>
          )}
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div
              className="space-y-3"
              aria-busy="true"
              aria-label="Loading holdings"
            >
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : isError ? (
            <div
              role="alert"
              className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-center"
            >
              <p className="text-sm">Couldn&apos;t load holdings.</p>
              <Button size="sm" variant="outline" onClick={() => refetch()}>
                Try again
              </Button>
            </div>
          ) : investments.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-8 text-center">
              <h3 className="text-lg font-semibold">No holdings yet</h3>
              <p className="text-muted-foreground mt-1 text-sm">
                Add your first investment to start tracking this account.
              </p>
              <Button
                className="mt-3"
                size="sm"
                onClick={() => setAddOpen(true)}
              >
                <Plus className="size-4" />
                Add investment
              </Button>
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Symbol</TableHead>
                    <TableHead className="hidden sm:table-cell">Name</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="hidden text-right sm:table-cell">
                      Price
                    </TableHead>
                    <TableHead className="text-right">Market value</TableHead>
                    <TableHead className="text-right">Gain/loss</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {investments.map((inv) => {
                    // Native figures stay native; base figures drive P&L and
                    // are never mixed in one calculation.
                    const nativeValue = Math.round(
                      inv.quantity * inv.current_price_native,
                    );
                    const baseValue = Math.round(
                      inv.quantity * inv.current_price,
                    );
                    const gainLossBase = baseValue - inv.cost_basis_base;
                    return (
                      <TableRow
                        key={inv.id}
                        className="cursor-pointer"
                        onClick={() => router.push(`/investments/${inv.id}`)}
                        onMouseEnter={() => prefetchInvestment(inv.id)}
                      >
                        <TableCell className="font-mono font-semibold">
                          <Link
                            href={`/investments/${inv.id}`}
                            onClick={(e) => e.stopPropagation()}
                          >
                            {inv.security.symbol}
                          </Link>
                        </TableCell>
                        <TableCell className="hidden sm:table-cell">
                          {inv.security.name}
                        </TableCell>
                        <TableCell className="text-right">
                          {inv.quantity}
                        </TableCell>
                        <TableCell className="hidden text-right sm:table-cell">
                          {formatMoney(
                            inv.current_price_native,
                            inv.price_currency,
                            hideBalances,
                          )}
                        </TableCell>
                        <TableCell className="text-right font-medium">
                          <Money
                            amount={nativeValue}
                            currency={inv.price_currency}
                            baseAmount={baseValue}
                            fxRate={inv.fx_rate}
                            fxStale={inv.fx_stale}
                            fxMissing={inv.fx_missing}
                            layout="stacked"
                            className="items-end"
                          />
                        </TableCell>
                        <TableCell
                          className={`text-right font-medium ${
                            inv.fx_missing
                              ? "text-muted-foreground"
                              : gainLossBase >= 0
                                ? "text-positive"
                                : "text-negative"
                          }`}
                        >
                          {inv.fx_missing ? (
                            "—"
                          ) : (
                            <Money
                              amount={gainLossBase}
                              currency={baseCurrency}
                              sign="always"
                            />
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>

              {totalPages > 1 && (
                <div className="mt-4 flex items-center justify-between">
                  <span className="text-muted-foreground text-sm">
                    Page {page} of {totalPages}
                  </span>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page <= 1}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      <ChevronLeft className="size-4" />
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page >= totalPages}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      Next
                      <ChevronRight className="size-4" />
                    </Button>
                  </div>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <EditAccountDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        account={account}
      />
      <AddInvestmentDialog
        accountId={account.id}
        open={addOpen}
        onOpenChange={setAddOpen}
      />
    </div>
  );
}
