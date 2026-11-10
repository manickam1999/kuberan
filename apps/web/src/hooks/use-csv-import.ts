import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { CsvImportReport } from "@/types/api";

export interface CsvImportInput {
  file: File;
  accountId: string;
  /** The mapping form fields (see mappingFormFields); omit to just inspect the file. */
  mapping?: Record<string, string>;
  /** false (default) is a dry run that writes nothing. */
  apply?: boolean;
}

/** Builds the multipart body of POST /imports/csv (see docs/csv-import.md). */
export function buildCsvImportForm(input: CsvImportInput): FormData {
  const form = new FormData();
  form.append("file", input.file);
  form.append("account_id", input.accountId);
  for (const [name, value] of Object.entries(input.mapping ?? {})) {
    if (value !== "") form.append(name, value);
  }
  if (input.apply) form.append("apply", "true");
  return form;
}

/**
 * Runs the CSV import: without a mapping it inspects the file, with one it
 * returns the dry-run report, and `apply: true` writes it. An apply changes
 * balances and lists, so everything is invalidated afterwards.
 */
export function useCsvImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CsvImportInput) =>
      apiClient.upload<CsvImportReport>(
        "/api/v1/imports/csv",
        buildCsvImportForm(input)
      ),
    onSuccess: (report) => {
      if (report.applied) void queryClient.invalidateQueries();
    },
  });
}
