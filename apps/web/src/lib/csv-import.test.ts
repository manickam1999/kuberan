import { describe, expect, it } from "vitest";
import { buildCsvImportForm } from "@/hooks/use-csv-import";
import {
  canApplyReport,
  formFromSuggestion,
  mappingFormFields,
  mappingReady,
  rowStatusLabel,
} from "@/lib/csv-import";
import type { CsvImportReport, CsvMapping } from "@/types/api";

const bank: CsvMapping = {
  date_column: "Transaction Date",
  description_column: "Details",
  debit_column: "Withdrawal",
  credit_column: "Deposit",
  date_format: "DD/MM/YYYY",
  decimal_separator: ".",
};

describe("formFromSuggestion", () => {
  it("preselects debit and credit columns when there is no amount column", () => {
    const f = formFromSuggestion(bank);
    expect(f.amountMode).toBe("split");
    expect(f.debitColumn).toBe("Withdrawal");
    expect(f.dateFormat).toBe("DD/MM/YYYY");
    expect(f.amountSign).toBe("negative_is_expense");
  });

  it("prefers the single amount column and keeps the card sign", () => {
    const f = formFromSuggestion({
      date_column: "Date",
      description_column: "Memo",
      amount_column: "Amount",
      date_format: "YYYY-MM-DD",
      decimal_separator: ",",
      amount_sign: "positive_is_expense",
    });
    expect(f.amountMode).toBe("single");
    expect(f.amountSign).toBe("positive_is_expense");
    expect(f.decimalSeparator).toBe(",");
  });

  it("copes with an empty suggestion", () => {
    const f = formFromSuggestion({
      date_column: "",
      description_column: "",
      date_format: "",
      decimal_separator: "",
    });
    expect(mappingReady(f)).toBe(false);
    expect(f.decimalSeparator).toBe(".");
  });
});

describe("mappingReady and mappingFormFields", () => {
  it("needs date, description and a way to read the amount", () => {
    const f = formFromSuggestion(bank);
    expect(mappingReady(f)).toBe(true);
    expect(mappingReady({ ...f, creditColumn: "" })).toBe(false);
    expect(mappingReady({ ...f, amountMode: "single" })).toBe(false);
    expect(mappingReady({ ...f, dateColumn: "" })).toBe(false);
    expect(mappingReady({ ...f, dateFormat: "" })).toBe(false);
  });

  it("sends only the fields that apply to the chosen amount layout", () => {
    const split = mappingFormFields(formFromSuggestion(bank));
    expect(split).toMatchObject({
      date_column: "Transaction Date",
      debit_column: "Withdrawal",
      credit_column: "Deposit",
      date_format: "DD/MM/YYYY",
      decimal_separator: ".",
    });
    expect(split).not.toHaveProperty("amount_column");
    expect(split).not.toHaveProperty("amount_sign");

    const single = mappingFormFields({
      ...formFromSuggestion(bank),
      amountMode: "single",
      amountColumn: "Amount",
      categoryColumn: "Category",
    });
    expect(single.amount_column).toBe("Amount");
    expect(single.amount_sign).toBe("negative_is_expense");
    expect(single.category_column).toBe("Category");
    expect(single).not.toHaveProperty("debit_column");
    expect(single).not.toHaveProperty("notes_column");
  });
});

describe("buildCsvImportForm", () => {
  const file = new File(["a,b\n1,2\n"], "statement.csv", { type: "text/csv" });

  it("inspects with just the file and the account", () => {
    const form = buildCsvImportForm({ file, accountId: "acc-1" });
    expect(form.get("account_id")).toBe("acc-1");
    expect(form.get("file")).toBeInstanceOf(File);
    expect(form.has("apply")).toBe(false);
    expect(form.has("date_column")).toBe(false);
  });

  it("adds the mapping and apply", () => {
    const form = buildCsvImportForm({
      file,
      accountId: "acc-1",
      mapping: { date_column: "Date", notes_column: "" },
      apply: true,
    });
    expect(form.get("date_column")).toBe("Date");
    expect(form.has("notes_column")).toBe(false);
    expect(form.get("apply")).toBe("true");
  });
});

describe("report helpers", () => {
  const report = (over: Partial<CsvImportReport>): CsvImportReport => ({
    applied: false,
    account: { id: "a", name: "Bank", currency: "MYR" },
    delimiter: ",",
    columns: [],
    sample: null,
    suggested_mapping: bank,
    created: 0,
    date_formats: [],
    ...over,
  });
  const summary = {
    rows: 3,
    would_create: 2,
    skip_duplicate: 1,
    skip_manual: 0,
    invalid: 0,
    categorized: 0,
    categorized_by_rule: 0,
    unknown_category: 0,
    income_total: 0,
    expense_total: 0,
  };

  it("labels row outcomes", () => {
    expect(rowStatusLabel("create")).toBe("Will import");
    expect(rowStatusLabel("skip_manual")).toBe("Matches your entry");
  });

  it("only allows applying a dry run that would create rows", () => {
    expect(canApplyReport(null)).toBe(false);
    expect(canApplyReport(report({}))).toBe(false);
    expect(canApplyReport(report({ summary }))).toBe(true);
    expect(canApplyReport(report({ summary, applied: true }))).toBe(false);
    expect(
      canApplyReport(report({ summary: { ...summary, would_create: 0 } }))
    ).toBe(false);
  });
});
