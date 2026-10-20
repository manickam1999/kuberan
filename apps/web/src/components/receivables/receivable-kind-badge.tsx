import { Badge } from "@/components/ui/badge";
import { RECEIVABLE_KIND_LABELS } from "@/lib/receivable-format";
import type { ReceivableKind } from "@/types/models";

/** Outline pill showing a receivable's kind (Split, Paid for someone, Claim, Loan, Deposit, Refund). */
export function ReceivableKindBadge({
  kind,
  className,
}: {
  kind: ReceivableKind;
  className?: string;
}) {
  return (
    <Badge variant="outline" className={className}>
      {RECEIVABLE_KIND_LABELS[kind]}
    </Badge>
  );
}
