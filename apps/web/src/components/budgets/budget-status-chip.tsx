import { Badge } from "@/components/ui/badge";
import type { BudgetStatus } from "@/lib/budget-pace";

const LABEL: Record<BudgetStatus, string> = {
  on_track: "On track",
  close: "Close",
  over: "Over",
};

/** On track / Close / Over chip. The word carries the meaning, not just colour. */
export function BudgetStatusChip({ status }: { status: BudgetStatus }) {
  return (
    <Badge
      tone={status === "on_track" ? "pos" : status === "close" ? "warn" : "neg"}
    >
      {LABEL[status]}
    </Badge>
  );
}
