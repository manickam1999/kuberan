"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import { useAccount } from "@/hooks/use-accounts";
import { PageSkeleton } from "@/components/layout/page-skeleton";
import { CashAccountView } from "@/components/accounts/cash-account-view";
import { CreditCardView } from "@/components/accounts/credit-card-view";
import { FixedDepositView } from "@/components/accounts/fixed-deposit-view";
import { InvestmentAccountView } from "@/components/accounts/investment-account-view";
import { LoanView } from "@/components/accounts/loan-view";
import { Button } from "@/components/ui/button";

/** Account detail: picks the screen for the account's type. */
export default function AccountDetailPage() {
  const params = useParams();
  const accountId = params.id as string;
  const { data: account, isLoading, isError, refetch } = useAccount(accountId);

  if (isLoading) return <PageSkeleton variant="detail" />;

  if (isError) {
    return (
      <div
        role="alert"
        className="flex flex-col items-center gap-3 py-12 text-center"
      >
        <h2 className="text-lg font-semibold">
          Couldn&apos;t load this account
        </h2>
        <p className="text-muted-foreground text-sm">
          Check your connection and try again.
        </p>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => refetch()}>
            Try again
          </Button>
          <Button asChild variant="ghost">
            <Link href="/accounts">Back to Accounts</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (!account) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <h2 className="text-lg font-semibold">Account not found</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          This account may have been deleted or you don&apos;t have access.
        </p>
        <Button asChild className="mt-4" variant="outline">
          <Link href="/accounts">Back to Accounts</Link>
        </Button>
      </div>
    );
  }

  switch (account.type) {
    case "credit_card":
      return <CreditCardView account={account} />;
    case "fixed_deposit":
      return <FixedDepositView account={account} />;
    case "investment":
      return <InvestmentAccountView account={account} />;
    case "debt":
      return <LoanView account={account} />;
    default:
      return <CashAccountView account={account} />;
  }
}
