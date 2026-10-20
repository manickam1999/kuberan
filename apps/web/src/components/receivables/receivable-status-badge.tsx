import { Badge } from "@/components/ui/badge";
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

/** Pill showing a receivable's status (Pending / Partially paid / Settled / Written off). */
export function ReceivableStatusBadge({
  status,
  className,
}: {
  status: ReceivableStatus;
  className?: string;
}) {
  return (
    <Badge variant={VARIANT[status]} className={className}>
      {RECEIVABLE_STATUS_LABELS[status]}
    </Badge>
  );
}
