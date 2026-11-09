import type { IbkrImportReport } from "@/types/api";

export interface IbkrReportSummary {
  errorCount: number;
  warningCount: number;
  /** Symbols whose replayed quantity disagrees with the broker's. */
  quantityMismatches: string[];
  securityCount: number;
  newTransactions: number;
  cashRows: number;
  /** The report allows an apply: no errors and every quantity checkpoint agrees. */
  canApply: boolean;
}

/** Condenses a report into what the UI needs to decide whether apply is safe. */
export function summarizeIbkrReport(report: IbkrImportReport): IbkrReportSummary {
  const securities = report.securities ?? [];
  const quantityMismatches = securities
    .filter((s) => (s.checkpoints ?? []).some((c) => !c.quantity_ok))
    .map((s) => s.symbol);
  const errorCount = (report.errors ?? []).length;
  return {
    errorCount,
    warningCount: (report.warnings ?? []).length,
    quantityMismatches,
    securityCount: securities.length,
    newTransactions: securities.reduce((n, s) => n + s.new_transactions, 0),
    cashRows: report.cash.legs + report.cash.dividends + report.cash.fees,
    canApply: errorCount === 0 && quantityMismatches.length === 0,
  };
}

/** Pulls the report out of a 400 IMPORT_INVALID error's `details`, if present. */
export function reportFromErrorDetails(
  details: Record<string, unknown> | undefined
): IbkrImportReport | null {
  const r = details?.report;
  if (r && typeof r === "object" && "cash" in r && "applied" in r) {
    return r as IbkrImportReport;
  }
  return null;
}
