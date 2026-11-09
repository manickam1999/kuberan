"use client";

import { useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, FileUp } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge, type ChipTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { useAccounts } from "@/hooks/use-accounts";
import { useCsvImport } from "@/hooks/use-csv-import";
import { ApiClientError } from "@/lib/api-client";
import {
  canApplyReport,
  formFromSuggestion,
  mappingFormFields,
  mappingReady,
  rowStatusLabel,
  type CsvMappingForm,
} from "@/lib/csv-import";
import type { CsvImportReport, CsvRowStatus } from "@/types/api";

const NONE = "none";

const ROW_TONE: Record<CsvRowStatus, ChipTone> = {
  create: "pos",
  skip_duplicate: "neutral",
  skip_manual: "warn",
  invalid: "neg",
};

const DATE_FORMAT_FALLBACK = ["YYYY-MM-DD", "DD/MM/YYYY", "MM/DD/YYYY"];

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-sm font-medium tabular-nums">{value}</p>
    </div>
  );
}

interface ColumnSelectProps {
  id: string;
  label: string;
  value: string;
  columns: string[];
  onChange: (v: string) => void;
  /** Allow "None" (optional columns). */
  optional?: boolean;
}

function ColumnSelect({
  id,
  label,
  value,
  columns,
  onChange,
  optional,
}: ColumnSelectProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select
        value={value || (optional ? NONE : undefined)}
        onValueChange={(v) => onChange(v === NONE ? "" : v)}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue placeholder="Choose column" />
        </SelectTrigger>
        <SelectContent>
          {optional && <SelectItem value={NONE}>None</SelectItem>}
          {columns.map((c, i) => (
            <SelectItem key={`${c}-${i}`} value={c}>
              {c || `(column ${i + 1})`}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function SamplePreview({ report }: { report: CsvImportReport }) {
  const sample = report.sample ?? [];
  if (sample.length === 0) return null;
  return (
    <div className="space-y-1.5">
      <p className="text-sm font-medium">First rows of the file</p>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b bg-muted/40 text-left text-muted-foreground">
              {report.columns.map((c, i) => (
                <th key={i} className="px-2.5 py-1.5 font-medium whitespace-nowrap">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sample.map((row, r) => (
              <tr key={r} className="border-b last:border-0">
                {report.columns.map((_, c) => (
                  <td key={c} className="max-w-48 truncate px-2.5 py-1.5 whitespace-nowrap">
                    {row[c] ?? ""}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ReportView({ report }: { report: CsvImportReport }) {
  const s = report.summary;
  const currency = report.account.currency;
  const issues = report.issues ?? [];
  const rows = report.rows ?? [];
  if (!s) return null;

  return (
    <div className="space-y-4" aria-live="polite">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={report.applied ? "pos" : "neutral"}>
          {report.applied
            ? `Imported ${report.created} transaction${report.created === 1 ? "" : "s"}`
            : "Dry run: nothing saved"}
        </Badge>
        <span className="text-xs text-muted-foreground">
          {report.account.name} · {currency}
          {s.first_date && s.last_date ? ` · ${s.first_date} to ${s.last_date}` : ""}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat label={report.applied ? "Imported" : "Will import"} value={report.applied ? report.created : s.would_create} />
        <Stat label="Already imported" value={s.skip_duplicate} />
        <Stat label="Match your entries" value={s.skip_manual} />
        <Stat label="Can't read" value={s.invalid} />
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Stat label="Money in" value={<Money amount={s.income_total} currency={currency} />} />
        <Stat label="Money out" value={<Money amount={s.expense_total} currency={currency} />} />
        <Stat label="Categorised" value={`${s.categorized} of ${s.would_create}`} />
        <Stat label="By your rules" value={s.categorized_by_rule} />
      </div>

      {s.unknown_category > 0 && (
        <p className="text-xs text-muted-foreground">
          {s.unknown_category} row{s.unknown_category === 1 ? "" : "s"} named a
          category you don&apos;t have; your rules were tried instead.
        </p>
      )}

      {issues.length > 0 && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>
            {s.invalid} row{s.invalid === 1 ? "" : "s"} can&apos;t be read and
            will be skipped
          </AlertTitle>
          <AlertDescription>
            <ul className="mt-1 list-disc space-y-1 pl-4">
              {issues.slice(0, 8).map((i) => (
                <li key={`${i.line}-${i.code}`}>
                  Line {i.line}: {i.detail}
                </li>
              ))}
              {issues.length > 8 && <li>and {issues.length - 8} more</li>}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {rows.length > 0 && (
        <div className="space-y-1.5">
          <p className="text-sm font-medium">Rows</p>
          <ul className="divide-y divide-border/60 rounded-lg border">
            {rows.slice(0, 25).map((r) => (
              <li
                key={r.line}
                className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm"
              >
                <span className="w-20 shrink-0 text-xs text-muted-foreground tabular-nums">
                  {r.date ?? `line ${r.line}`}
                </span>
                <span className="min-w-0 flex-1 truncate">
                  {r.description ?? r.reason}
                </span>
                {r.category && (
                  <span className="text-xs text-muted-foreground">
                    {r.category}
                    {r.category_source === "rule" ? " (rule)" : ""}
                  </span>
                )}
                {r.amount ? (
                  <span className="text-sm font-medium">
                    <Money
                      amount={r.type === "expense" ? -r.amount : r.amount}
                      currency={currency}
                    />
                  </span>
                ) : null}
                <Badge tone={ROW_TONE[r.status]} title={r.reason}>
                  {rowStatusLabel(r.status)}
                </Badge>
              </li>
            ))}
          </ul>
          {(rows.length > 25 || report.rows_truncated) && (
            <p className="text-xs text-muted-foreground">
              Showing the first 25 of {s.rows} rows (problems first).
            </p>
          )}
        </div>
      )}
    </div>
  );
}

/** Settings > Data: import a bank-statement CSV (inspect, map, dry run, apply). */
export function CsvImport() {
  const { data: accountsData } = useAccounts({ page_size: 100 });
  const importer = useCsvImport();
  const fileRef = useRef<HTMLInputElement>(null);

  const [file, setFile] = useState<File | null>(null);
  const [accountId, setAccountId] = useState("");
  const [inspected, setInspected] = useState<CsvImportReport | null>(null);
  const [form, setForm] = useState<CsvMappingForm | null>(null);
  const [report, setReport] = useState<CsvImportReport | null>(null);
  // What the shown dry-run report was computed from; any edit invalidates it.
  const [reportKey, setReportKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const accounts = (accountsData?.data ?? []).filter(
    (a) =>
      a.is_active &&
      (a.type === "cash" || a.type === "credit_card" || a.type === "debt")
  );
  const columns = inspected?.columns ?? [];
  const key = useMemo(
    () =>
      JSON.stringify([
        file ? [file.name, file.size, file.lastModified] : null,
        accountId,
        form,
      ]),
    [file, accountId, form]
  );
  const fresh = reportKey === key;
  const canApply = canApplyReport(report) && fresh;

  function patch(p: Partial<CsvMappingForm>) {
    setForm((f) => (f ? { ...f, ...p } : f));
  }

  function reset(nextFile: File | null) {
    setFile(nextFile);
    setInspected(null);
    setForm(null);
    setReport(null);
    setReportKey(null);
    setError(null);
  }

  function describeError(err: unknown): string {
    if (err instanceof ApiClientError) {
      if (err.code === "IMPORT_IN_PROGRESS") {
        return "Another import is being applied. Try again in a moment.";
      }
      if (err.code === "CSV_TOO_MANY_ROWS") {
        return "That file has more than 5,000 rows. Split it into smaller files.";
      }
      if (err.status === 413) return "That file is too large.";
      return err.message;
    }
    return "Something went wrong. Please try again.";
  }

  function readFile() {
    if (!file || !accountId) return;
    setError(null);
    importer.mutate(
      { file, accountId },
      {
        onSuccess: (r) => {
          setInspected(r);
          setForm(formFromSuggestion(r.suggested_mapping));
          setReport(null);
          setReportKey(null);
        },
        onError: (err) => setError(describeError(err)),
      }
    );
  }

  function run(apply: boolean) {
    if (!file || !form) return;
    setError(null);
    importer.mutate(
      { file, accountId, mapping: mappingFormFields(form), apply },
      {
        onSuccess: (r) => {
          setReport(r);
          setReportKey(key);
          if (r.applied) {
            toast.success(
              `Imported ${r.created} transaction${r.created === 1 ? "" : "s"}`
            );
            if (fileRef.current) fileRef.current.value = "";
            setFile(null);
            setInspected(null);
            setForm(null);
          }
        },
        onError: (err) => setError(describeError(err)),
      }
    );
  }

  const dateFormats = inspected?.date_formats?.length
    ? inspected.date_formats
    : DATE_FORMAT_FALLBACK;
  const toCreate = report?.summary?.would_create ?? 0;
  const applyLabel =
    toCreate > 0
      ? `Import ${toCreate} transaction${toCreate === 1 ? "" : "s"}`
      : "Import";
  const pendingInspect = importer.isPending && !importer.variables?.mapping;

  return (
    <SettingsSection
      title="Import a bank CSV"
      description="Add the rows of a bank or card statement to one account. You'll map the columns and see exactly what would be added; nothing is saved until you apply it."
    >
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="csv-file">Statement (CSV, up to 5,000 rows)</Label>
            <Input
              id="csv-file"
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              onChange={(e) => reset(e.target.files?.[0] ?? null)}
            />
            <p className="text-xs text-muted-foreground">
              Read in memory and never stored. UTF-8 with a header row.
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="csv-account">Account</Label>
            <Select
              value={accountId || undefined}
              onValueChange={(v) => {
                setAccountId(v);
                setInspected(null);
                setForm(null);
                setReport(null);
                setReportKey(null);
              }}
            >
              <SelectTrigger id="csv-account" className="w-full">
                <SelectValue placeholder="Choose account" />
              </SelectTrigger>
              <SelectContent>
                {accounts.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.name} · {a.currency}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Amounts are taken in this account&apos;s currency.
            </p>
          </div>
        </div>

        {!inspected && (
          <Button
            onClick={readFile}
            disabled={!file || !accountId || importer.isPending}
          >
            <FileUp className="size-4" />
            {pendingInspect ? "Reading..." : "Read the file"}
          </Button>
        )}

        {inspected && form && (
          <div className="space-y-4">
            <SamplePreview report={inspected} />

            <div className="space-y-3">
              <p className="text-sm font-medium">
                Which column is which?{" "}
                <span className="font-normal text-muted-foreground">
                  We guessed from the headers; change anything that&apos;s off.
                </span>
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                <ColumnSelect
                  id="csv-date"
                  label="Date"
                  value={form.dateColumn}
                  columns={columns}
                  onChange={(v) => patch({ dateColumn: v })}
                />
                <ColumnSelect
                  id="csv-desc"
                  label="Description"
                  value={form.descriptionColumn}
                  columns={columns}
                  onChange={(v) => patch({ descriptionColumn: v })}
                />
              </div>

              <div className="space-y-2">
                <Label>Amounts are in</Label>
                <Segmented
                  aria-label="Amount layout"
                  value={form.amountMode}
                  onValueChange={(v) => patch({ amountMode: v })}
                  options={[
                    { value: "single", label: "One signed column" },
                    { value: "split", label: "Debit and credit columns" },
                  ]}
                />
              </div>
              {form.amountMode === "single" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <ColumnSelect
                    id="csv-amount"
                    label="Amount"
                    value={form.amountColumn}
                    columns={columns}
                    onChange={(v) => patch({ amountColumn: v })}
                  />
                  <div className="space-y-1.5">
                    <Label htmlFor="csv-sign">Money out is</Label>
                    <Select
                      value={form.amountSign}
                      onValueChange={(v) =>
                        patch({
                          amountSign:
                            v === "positive_is_expense"
                              ? "positive_is_expense"
                              : "negative_is_expense",
                        })
                      }
                    >
                      <SelectTrigger id="csv-sign" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="negative_is_expense">
                          Negative (most bank statements)
                        </SelectItem>
                        <SelectItem value="positive_is_expense">
                          Positive (most card statements)
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              ) : (
                <div className="grid gap-4 sm:grid-cols-2">
                  <ColumnSelect
                    id="csv-debit"
                    label="Money out (debit)"
                    value={form.debitColumn}
                    columns={columns}
                    onChange={(v) => patch({ debitColumn: v })}
                  />
                  <ColumnSelect
                    id="csv-credit"
                    label="Money in (credit)"
                    value={form.creditColumn}
                    columns={columns}
                    onChange={(v) => patch({ creditColumn: v })}
                  />
                </div>
              )}

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="csv-date-format">Date format</Label>
                  <Select
                    value={form.dateFormat || undefined}
                    onValueChange={(v) => patch({ dateFormat: v })}
                  >
                    <SelectTrigger id="csv-date-format" className="w-full">
                      <SelectValue placeholder="Choose format" />
                    </SelectTrigger>
                    <SelectContent>
                      {dateFormats.map((f) => (
                        <SelectItem key={f} value={f}>
                          {f}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="csv-decimal">Decimal separator</Label>
                  <Select
                    value={form.decimalSeparator}
                    onValueChange={(v) => patch({ decimalSeparator: v })}
                  >
                    <SelectTrigger id="csv-decimal" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value=".">Dot (1,234.50)</SelectItem>
                      <SelectItem value=",">Comma (1.234,50)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <ColumnSelect
                  id="csv-category"
                  label="Category (optional)"
                  value={form.categoryColumn}
                  columns={columns}
                  optional
                  onChange={(v) => patch({ categoryColumn: v })}
                />
                <ColumnSelect
                  id="csv-notes"
                  label="Notes (optional)"
                  value={form.notesColumn}
                  columns={columns}
                  optional
                  onChange={(v) => patch({ notesColumn: v })}
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Rows already imported, and rows that match something you typed
                in by hand, are skipped. Your rules categorise the rest.
              </p>
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => run(false)}
                disabled={!mappingReady(form) || importer.isPending}
              >
                <FileUp className="size-4" />
                {importer.isPending && !importer.variables?.apply && !pendingInspect
                  ? "Checking..."
                  : "Preview import (dry run)"}
              </Button>
              <Button
                variant="outline"
                onClick={() => run(true)}
                disabled={!canApply || importer.isPending}
              >
                {importer.isPending && importer.variables?.apply
                  ? "Applying..."
                  : applyLabel}
              </Button>
            </div>
            {report && !report.applied && !fresh && (
              <p className="text-xs text-muted-foreground">
                You changed the mapping since this report. Check the import
                again before applying.
              </p>
            )}
          </div>
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
