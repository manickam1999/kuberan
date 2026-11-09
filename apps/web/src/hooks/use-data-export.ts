import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { exportFileName } from "@/lib/export-file";

/**
 * Fetch the data export (a zip of CSVs) with the Bearer token and save it
 * through a temporary object URL. A plain link could not authenticate.
 * Rejects with ApiClientError (429 RATE_LIMITED when exported within the last
 * minute).
 */
export async function downloadDataExport(
  date: Date = new Date()
): Promise<void> {
  const blob = await apiClient.getBlob("/api/v1/export");
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = exportFileName(date);
    document.body.appendChild(link);
    link.click();
    link.remove();
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Mutation wrapper around downloadDataExport for a button's pending state. */
export function useDataExport() {
  return useMutation({ mutationFn: () => downloadDataExport() });
}
