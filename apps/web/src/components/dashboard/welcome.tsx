"use client";

import Link from "next/link";
import { Check, ChevronRight } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { Tile } from "@/components/ui/tile";
import { Eyebrow, HomeCard } from "@/components/dashboard/home-bits";
import { SampleDataCard } from "@/components/dashboard/sample-data-card";
import { accountVisual } from "@/lib/domain-visuals";
import { useBaseCurrency } from "@/hooks/use-profile";
import { cn } from "@/lib/utils";
import type { OnboardingStep } from "@/lib/home";
import type { Account } from "@/types/models";

/** Where each step goes. The first two open the same dialogs as "New". */
type StepTarget =
  | { kind: "action"; run: () => void }
  | { kind: "link"; href: string };

interface WelcomeProps {
  firstName?: string;
  steps: readonly OnboardingStep[];
  accounts: readonly Account[];
  onAddAccount: () => void;
  onAddTransaction: () => void;
}

/** Four-step checklist shown on Home until there is an account with real activity. */
export function Welcome({
  firstName,
  steps,
  accounts,
  onAddAccount,
  onAddTransaction,
}: WelcomeProps) {
  const baseCurrency = useBaseCurrency();
  const doneCount = steps.filter((s) => s.done).length;
  const hasAccount = steps[0]?.done ?? false;

  const targets: Record<OnboardingStep["id"], StepTarget> = {
    account: { kind: "action", run: onAddAccount },
    // A transaction needs an account to land in, so step 2 waits for step 1.
    transaction: hasAccount
      ? { kind: "action", run: onAddTransaction }
      : { kind: "action", run: onAddAccount },
    budget: { kind: "link", href: "/budgets" },
    assistant: { kind: "link", href: "/settings?tab=connected-apps" },
  };

  return (
    <div className="space-y-5">
      <div>
        <Eyebrow>Welcome</Eyebrow>
        <h1 className="font-display text-3xl leading-tight max-md:hidden">
          {firstName ? `Let’s set up Kuberan, ${firstName}` : "Let’s set up Kuberan"}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Four steps, about five minutes. Everything stays on this server.
        </p>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
        <Card className="gap-0 py-0">
          <div className="flex items-center gap-3 border-b px-5 py-4">
            <h2 className="text-sm font-semibold">Getting started</h2>
            <span className="ml-auto text-sm tabular-nums text-muted-foreground">
              {doneCount} of {steps.length}
            </span>
          </div>
          <div
            role="progressbar"
            aria-label="Setup progress"
            aria-valuemin={0}
            aria-valuemax={steps.length}
            aria-valuenow={doneCount}
            className="mx-5 mt-4 h-1.5 overflow-hidden rounded-full bg-muted"
          >
            <div
              className="h-full rounded-full bg-primary transition-[width]"
              style={{ width: `${(doneCount / steps.length) * 100}%` }}
            />
          </div>
          <ol className="p-2 pt-3">
            {steps.map((step, i) => {
              const target = targets[step.id];
              const body = (
                <>
                  <span
                    className={cn(
                      "grid size-7 shrink-0 place-items-center rounded-full border text-xs font-medium tabular-nums",
                      step.done
                        ? "border-transparent bg-positive-muted text-positive-text"
                        : "text-muted-foreground"
                    )}
                  >
                    {step.done ? (
                      <Check className="size-3.5" aria-hidden />
                    ) : (
                      i + 1
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span
                      className={cn(
                        "block text-sm font-medium",
                        step.done && "text-muted-foreground line-through"
                      )}
                    >
                      {step.title}
                      {step.done && <span className="sr-only"> (done)</span>}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {step.description}
                    </span>
                  </span>
                  <ChevronRight
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden
                  />
                </>
              );
              const rowClass =
                "flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-accent/50 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none";
              return (
                <li key={step.id}>
                  {target.kind === "link" ? (
                    <Link href={target.href} className={rowClass}>
                      {body}
                    </Link>
                  ) : (
                    <button
                      type="button"
                      onClick={target.run}
                      className={rowClass}
                    >
                      {body}
                    </button>
                  )}
                </li>
              );
            })}
          </ol>
        </Card>

        <div className="space-y-4">
          <SampleDataCard />
          {accounts.length > 0 && (
            <HomeCard title="Your accounts" href="/accounts" linkLabel="Manage">
              <div className="space-y-0.5">
                {accounts.slice(0, 5).map((a) => {
                  const v = accountVisual(a.type);
                  const Icon = v.icon;
                  return (
                    <Link
                      key={a.id}
                      href={`/accounts/${a.id}`}
                      className="-mx-2 flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-accent/50 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                    >
                      <Tile tone="accent">
                        <Icon />
                      </Tile>
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">
                        {a.name}
                      </span>
                      <span className="shrink-0 text-sm font-medium">
                        <Money
                          amount={a.balance}
                          currency={a.currency}
                          baseAmount={a.balance_base}
                          baseCurrency={baseCurrency}
                          fxRate={a.fx_rate}
                          fxStale={a.fx_stale}
                          fxMissing={a.fx_missing}
                          layout="stacked"
                        />
                      </span>
                    </Link>
                  );
                })}
              </div>
            </HomeCard>
          )}
        </div>
      </div>
    </div>
  );
}
