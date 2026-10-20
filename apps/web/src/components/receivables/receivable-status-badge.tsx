import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { RECEIVABLE_STATUS_LABELS } from "@/lib/receivable-format";
import type { ReceivableStatus } from "@/types/models";

const VARIANT: Record<
  ReceivableStatus,
  "warning" | "info" | "positive" | "secondary"
> = {
  pending: "warning",
  partially_paid: "info",
  settled: "positive",
  written_off: "secondary",
};

// Darker light-mode text (AA at 12px on the muted fills); equals the base tone in dark mode.
const TEXT: Record<ReceivableStatus, string> = {
  pending: "text-warning-text",
  partially_paid: "text-info-text",
  settled: "text-positive-text",
  written_off: "",
};

/** Pill showing a receivable's status (Pending / Partially paid / Settled / Written off). */
export function ReceivableStatusBadge({
  status,
  className,
}: {
  status: ReceivableStatus;
  className?: string;
}) {
  return (
    <Badge variant={VARIANT[status]} className={cn(TEXT[status], className)}>
      {RECEIVABLE_STATUS_LABELS[status]}
    </Badge>
  );
}
