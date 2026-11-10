/** Download name for a data export taken on `date`: kuberan-export-YYYY-MM-DD.zip. */
export function exportFileName(date: Date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `kuberan-export-${y}-${m}-${d}.zip`;
}
