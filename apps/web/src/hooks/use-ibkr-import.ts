import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { IbkrImportReport } from "@/types/api";

export interface IbkrImportInput {
  files: File[];
  investmentAccountId: string;
  cashAccountId: string;
  /** YYYY-MM-DD */
  cashStartDate: string;
  legacyCashAccountId?: string;
  deactivateLegacyCashAccount?: boolean;
  /** false (default) is a dry run that writes nothing. */
  apply?: boolean;
}

/** Builds the multipart body of POST /imports/ibkr (see docs/ibkr-import.md). */
export function buildIbkrImportForm(input: IbkrImportInput): FormData {
  const form = new FormData();
  for (const file of input.files) form.append("files", file);
  form.append("investment_account_id", input.investmentAccountId);
  form.append("cash_account_id", input.cashAccountId);
  form.append("cash_start_date", input.cashStartDate);
  if (input.legacyCashAccountId) {
    form.append("legacy_cash_account_id", input.legacyCashAccountId);
    if (input.deactivateLegacyCashAccount) {
      form.append("deactivate_legacy_cash_account", "true");
    }
  }
  if (input.apply) form.append("apply", "true");
  return form;
}

/**
 * Runs the IBKR import: a dry run returns the report only; `apply: true`
 * writes it. Errors are ApiClientError; a refused apply is 400 IMPORT_INVALID
 * with the report in `details.report`. An apply changes balances and holdings,
 * so everything is invalidated afterwards.
 */
export function useIbkrImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: IbkrImportInput) =>
      apiClient.upload<IbkrImportReport>(
        "/api/v1/imports/ibkr",
        buildIbkrImportForm(input)
      ),
    onSuccess: (report) => {
      if (report.applied) void queryClient.invalidateQueries();
    },
  });
}
