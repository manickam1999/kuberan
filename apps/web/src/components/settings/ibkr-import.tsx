"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, Check, FileUp, X } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Money } from "@/components/ui/money";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { SettingsSection } from "@/components/settings/settings-section";
import { useAccounts } from "@/hooks/use-accounts";
import { useIbkrImport, type IbkrImportInput } from "@/hooks/use-ibkr-import";
import { ApiClientError } from "@/lib/api-client";
import { reportFromErrorDetails, summarizeIbkrReport } from "@/lib/ibkr-report";
import type { IbkrImportReport } from "@/types/api";

const MAX_FILES = 10;
const NONE = "none";

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-sm font-medium tabular-nums">{value}</p>
    </div>
  );
}

function ReportView({ report }: { report: IbkrImportReport }) {
  const s = summarizeIbkrReport(report);
  const errors = report.errors ?? [];
  const warnings = report.warnings ?? [];
  const securities = report.securities ?? [];
  const unmatched = report.cash.unmatched_deposits ?? [];
  const cashChecks = report.cash.checkpoints ?? [];

  return (
    <div className="space-y-4" aria-live="polite">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={report.applied ? "pos" : "neutral"}>
          {report.applied ? "Applied" : "Dry run: nothing saved"}
        </Badge>
        <span className="text-xs text-muted-foreground">
          Account {report.account} · statements in {report.base_currency}
          {report.periods && report.periods.length > 0
            ? ` · ${report.periods[0].start} to ${report.periods[report.periods.length - 1].end}`
            : ""}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat label="Holdings" value={s.securityCount} />
        <Stat label="Trades to write" value={s.newTransactions} />
        <Stat label="Cash rows" value={s.cashRows} />
        <Stat
          label="Opening cash"
          value={
            <Money
              amount={report.cash.opening_balance}
              currency={report.base_currency}
            />
          }
        />
      </div>

      {errors.length > 0 && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>
            {errors.length} problem{errors.length === 1 ? "" : "s"} to fix
            before applying
          </AlertTitle>
          <AlertDescription>
            <ul className="mt-1 list-disc space-y-1 pl-4">
              {errors.map((e, i) => (
                <li key={i}>
                  <code className="rounded bg-muted px-1 text-xs">{e.code}</code>{" "}
                  {e.detail}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {s.quantityMismatches.length > 0 && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>Quantities don&apos;t match IBKR</AlertTitle>
          <AlertDescription>
            {s.quantityMismatches.join(", ")}: the replayed quantity differs
            from IBKR&apos;s Open Positions, so the import can&apos;t be
            applied.
          </AlertDescription>
        </Alert>
      )}

      {warnings.length > 0 && (
        <div className="space-y-1">
          <p className="text-sm font-medium">Warnings</p>
          <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            {warnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </div>
      )}

      {securities.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm font-medium">Holdings</p>
          <ul className="divide-y divide-border/60 rounded-lg border">
            {securities.map((sec) => {
              const ok = (sec.checkpoints ?? []).every((c) => c.quantity_ok);
              return (
                <li
                  key={sec.security_id}
                  className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
                >
                  <span className="font-medium">{sec.symbol}</span>
                  <span className="text-xs text-muted-foreground">
                    {sec.new_transactions} trade
                    {sec.new_transactions === 1 ? "" : "s"}
                    {sec.investment_id ? "" : " · new holding"}
                  </span>
                  <Badge tone={ok ? "pos" : "neg"}>
                    {ok ? <Check /> : <X />}
                    {ok ? "Matches IBKR" : "Mismatch"}
                  </Badge>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {cashChecks.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm font-medium">Cash against IBKR&apos;s ending cash</p>
          <ul className="divide-y divide-border/60 rounded-lg border">
            {cashChecks.map((c) => (
              <li
                key={c.date}
                className="flex flex-wrap items-center justify-between gap-x-3 px-3 py-2 text-sm"
              >
                <span>{c.date}</span>
                <span className="text-xs text-muted-foreground">
                  IBKR{" "}
                  <Money amount={c.ibkr} currency={report.base_currency} /> ·
                  ledger{" "}
                  <Money amount={c.ledger} currency={report.base_currency} />
                </span>
                {c.diff !== 0 && (
                  <span className="text-xs">
                    difference{" "}
                    <Money
                      amount={c.diff}
                      currency={report.base_currency}
                      sign="always"
                    />
                  </span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      {unmatched.length > 0 && (
        <div className="space-y-1">
          <p className="text-sm font-medium">
            Deposits and withdrawals to record by hand
          </p>
          <ul className="space-y-1 text-sm text-muted-foreground">
            {unmatched.map((u, i) => (
              <li key={i}>
                {u.date} · {u.kind}{" "}
                <Money amount={u.amount} currency={report.base_currency} />
                {u.detail ? ` · ${u.detail}` : ""}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

/** Settings > Data: import IBKR Activity Statements (dry run first, then apply). */
export function IbkrImport() {
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const importer = useIbkrImport();
  const fileRef = useRef<HTMLInputElement>(null);

  const [files, setFiles] = useState<File[]>([]);
  const [investmentId, setInvestmentId] = useState("");
  const [cashId, setCashId] = useState("");
  const [legacyId, setLegacyId] = useState("");
  const [deactivateLegacy, setDeactivateLegacy] = useState(false);
  const [startDate, setStartDate] = useState("");
  const [report, setReport] = useState<IbkrImportReport | null>(null);
  // What the shown dry-run report was computed from; any edit invalidates it.
  const [reportKey, setReportKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const accounts = (accountsData?.data ?? []).filter((a) => a.is_active);
  const investmentAccounts = accounts.filter((a) => a.type === "investment");
  const cashAccounts = accounts.filter((a) => a.type === "cash");

  const input: IbkrImportInput = {
    files,
    investmentAccountId: investmentId,
    cashAccountId: cashId,
    cashStartDate: startDate,
    legacyCashAccountId: legacyId || undefined,
    deactivateLegacyCashAccount: legacyId ? deactivateLegacy : false,
  };
  const key = JSON.stringify([
    files.map((f) => [f.name, f.size, f.lastModified]),
    investmentId,
    cashId,
    legacyId,
    deactivateLegacy,
    startDate,
  ]);
  const complete =
    files.length > 0 && investmentId !== "" && cashId !== "" && startDate !== "";
  const summary = report ? summarizeIbkrReport(report) : null;
  const fresh = reportKey === key;
  const canApply =
    !!report && !report.applied && fresh && !!summary?.canApply;

  function describeError(err: unknown): string {
    if (err instanceof ApiClientError) {
      if (err.code === "IMPORT_IN_PROGRESS") {
        return "Another import is being applied. Try again in a moment.";
      }
      if (err.status === 413) return "One of the files is too large.";
      return err.message;
    }
    return "Something went wrong. Please try again.";
  }

  function run(apply: boolean) {
    setError(null);
    importer.mutate(
      { ...input, apply },
      {
        onSuccess: (r) => {
          setReport(r);
          setReportKey(key);
          if (r.applied) {
            toast.success("Import applied");
            setFiles([]);
            if (fileRef.current) fileRef.current.value = "";
          }
        },
        onError: (err) => {
          if (err instanceof ApiClientError && err.code === "IMPORT_INVALID") {
            const r = reportFromErrorDetails(err.details);
            if (r) {
              setReport(r);
              setReportKey(key);
            }
          }
          setError(describeError(err));
        },
      }
    );
  }

  return (
    <SettingsSection
      title="Import from Interactive Brokers"
      description="Rebuild your IBKR holdings and cash ledger from Activity Statement CSVs. You'll see a report first; nothing is saved until you apply it."
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="ibkr-files">Activity Statements (CSV, up to {MAX_FILES})</Label>
          <Input
            id="ibkr-files"
            ref={fileRef}
            type="file"
            accept=".csv,text/csv"
            multiple
            onChange={(e) =>
              setFiles(Array.from(e.target.files ?? []).slice(0, MAX_FILES))
            }
          />
          <p className="text-xs text-muted-foreground">
            One file per period for the same account, back to back. Files are
            read in memory and never stored.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="ibkr-investment">Investment account</Label>
            <Select value={investmentId} onValueChange={setInvestmentId}>
              <SelectTrigger id="ibkr-investment" className="w-full">
                <SelectValue placeholder="Choose account" />
              </SelectTrigger>
              <SelectContent>
                {investmentAccounts.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.name} · {a.currency}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="ibkr-cash">Cash account</Label>
            <Select value={cashId} onValueChange={setCashId}>
              <SelectTrigger id="ibkr-cash" className="w-full">
                <SelectValue placeholder="Choose account" />
              </SelectTrigger>
              <SelectContent>
                {cashAccounts.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.name} · {a.currency}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              In the statements&apos; base currency (usually USD).
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="ibkr-start">Cash starts on</Label>
            <Input
              id="ibkr-start"
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              A statement must end the day before; its ending cash becomes the
              opening balance.
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="ibkr-legacy">Old IBKR cash account (optional)</Label>
            <Select
              value={legacyId || NONE}
              onValueChange={(v) => setLegacyId(v === NONE ? "" : v)}
            >
              <SelectTrigger id="ibkr-legacy" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>None</SelectItem>
                {cashAccounts
                  .filter((a) => a.id !== cashId)
                  .map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.name} · {a.currency}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Transfers into it are re-pointed at the cash account above.
            </p>
          </div>
        </div>

        {legacyId && (
          <div className="flex items-center gap-3">
            <Switch
              id="ibkr-deactivate"
              checked={deactivateLegacy}
              onCheckedChange={setDeactivateLegacy}
            />
            <Label htmlFor="ibkr-deactivate">
              Deactivate the old account after applying
            </Label>
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          <Button
            onClick={() => run(false)}
            disabled={!complete || importer.isPending}
          >
            <FileUp className="size-4" />
            {importer.isPending && !importer.variables?.apply
              ? "Checking..."
              : "Check import (dry run)"}
          </Button>
          <Button
            variant="outline"
            onClick={() => run(true)}
            disabled={!canApply || importer.isPending}
          >
            {importer.isPending && importer.variables?.apply
              ? "Applying..."
              : "Apply import"}
          </Button>
        </div>
        {report && !report.applied && !fresh && (
          <p className="text-xs text-muted-foreground">
            You changed the form since this report. Check the import again
            before applying.
          </p>
        )}

        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}

        {report && <ReportView report={report} />}
      </div>
    </SettingsSection>
  );
}
