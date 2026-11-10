import type {
  CsvAmountSign,
  CsvImportReport,
  CsvMapping,
  CsvRowStatus,
} from "@/types/api";

/** The mapping fields the form edits; empty string means "not mapped". */
export interface CsvMappingForm {
  dateColumn: string;
  descriptionColumn: string;
  /** "single" uses one signed amount column, "split" a debit and a credit column. */
  amountMode: "single" | "split";
  amountColumn: string;
  debitColumn: string;
  creditColumn: string;
  categoryColumn: string;
  notesColumn: string;
  dateFormat: string;
  decimalSeparator: string;
  amountSign: CsvAmountSign;
}

/** Preselects the form from the server's suggested mapping. */
export function formFromSuggestion(m: CsvMapping): CsvMappingForm {
  const split = !m.amount_column && !!m.debit_column && !!m.credit_column;
  return {
    dateColumn: m.date_column ?? "",
    descriptionColumn: m.description_column ?? "",
    amountMode: split ? "split" : "single",
    amountColumn: m.amount_column ?? "",
    debitColumn: m.debit_column ?? "",
    creditColumn: m.credit_column ?? "",
    categoryColumn: m.category_column ?? "",
    notesColumn: m.notes_column ?? "",
    dateFormat: m.date_format ?? "",
    decimalSeparator: m.decimal_separator || ".",
    amountSign: m.amount_sign ?? "negative_is_expense",
  };
}

/** True when the required columns and the date format are chosen. */
export function mappingReady(f: CsvMappingForm): boolean {
  if (!f.dateColumn || !f.descriptionColumn || !f.dateFormat) return false;
  return f.amountMode === "single"
    ? !!f.amountColumn
    : !!f.debitColumn && !!f.creditColumn;
}

/** The form fields the API receives (only the ones that apply). */
export function mappingFormFields(f: CsvMappingForm): Record<string, string> {
  const out: Record<string, string> = {
    date_column: f.dateColumn,
    description_column: f.descriptionColumn,
  };
  if (f.amountMode === "single") {
    out.amount_column = f.amountColumn;
    out.amount_sign = f.amountSign;
  } else {
    out.debit_column = f.debitColumn;
    out.credit_column = f.creditColumn;
  }
  if (f.categoryColumn) out.category_column = f.categoryColumn;
  if (f.notesColumn) out.notes_column = f.notesColumn;
  if (f.dateFormat) out.date_format = f.dateFormat;
  if (f.decimalSeparator) out.decimal_separator = f.decimalSeparator;
  return out;
}

/** Plain-language label of a row outcome. */
export function rowStatusLabel(status: CsvRowStatus): string {
  switch (status) {
    case "create":
      return "Will import";
    case "skip_duplicate":
      return "Already imported";
    case "skip_manual":
      return "Matches your entry";
    case "invalid":
      return "Can't read";
  }
}

/** Whether applying the report would write anything. */
export function canApplyReport(report: CsvImportReport | null): boolean {
  return !!report && !report.applied && (report.summary?.would_create ?? 0) > 0;
}
