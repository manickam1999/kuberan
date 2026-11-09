"use client";

import { useState } from "react";
import { Eraser, History, MoreHorizontal, Pencil, Undo2 } from "lucide-react";
import { toast } from "sonner";
import { useUndoWriteOff } from "@/hooks/use-receivables";
import { formatDate } from "@/lib/format";
import {
  RECEIVABLE_KIND_LABELS,
  isOpen,
  receivableApiErrorMessage,
} from "@/lib/receivable-format";
import { avatarSlot, receivableDate, type PersonGroup } from "@/lib/owed";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Money } from "@/components/ui/money";
import { ReceivableSettlementHistory } from "./receivable-settlement-history";
import { SettleReceivableDialog } from "./settle-receivable-dialog";
import { WriteOffReceivableDialog } from "./write-off-receivable-dialog";
import type { Receivable } from "@/types/models";

/** Initial on a tint of one of the chart colours, stable per person. */
export function PersonAvatar({
  name,
  personKey,
  size = "md",
}: {
  name: string;
  personKey: string;
  size?: "md" | "sm";
}) {
  const slot = avatarSlot(personKey);
  return (
    <span
      aria-hidden
      className={
        size === "sm"
          ? "grid size-8 shrink-0 place-items-center rounded-full text-xs font-semibold"
          : "grid size-10 shrink-0 place-items-center rounded-full text-sm font-semibold"
      }
      style={{
        color: `var(--chart-${slot})`,
        backgroundColor: `color-mix(in oklab, var(--chart-${slot}) 18%, transparent)`,
      }}
    >
      {name.trim().charAt(0).toUpperCase() || "?"}
    </span>
  );
}

function ItemRow({ receivable }: { receivable: Receivable }) {
  const [settleOpen, setSettleOpen] = useState(false);
  const [writeOffOpen, setWriteOffOpen] = useState(false);
  const [undoOpen, setUndoOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const undoWriteOff = useUndoWriteOff();

  const open = isOpen(receivable);
  const hasPayments = receivable.paid_amount > 0;
  const pct = Math.min(100, (receivable.paid_amount / receivable.amount) * 100);
  const title =
    receivable.transaction?.description ||
    RECEIVABLE_KIND_LABELS[receivable.kind];

  async function undo() {
    try {
      await undoWriteOff.mutateAsync(receivable.id);
      toast.success("Write-off undone");
    } catch (err) {
      toast.error(receivableApiErrorMessage(err));
    }
  }

  const hasMenu = open || hasPayments || receivable.written_off_amount > 0;

  return (
    <div className="space-y-2">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{title}</p>
          <p className="text-muted-foreground text-xs">
            {RECEIVABLE_KIND_LABELS[receivable.kind]} ·{" "}
            {formatDate(receivableDate(receivable))}
            {!open && (
              <>
                {" · "}
                {receivable.status === "written_off" ? "Written off" : "Settled"}
              </>
            )}
          </p>
        </div>
        {open ? (
          <Money
            amount={receivable.outstanding}
            currency={receivable.currency}
            sign="never"
            className="money shrink-0 text-sm font-semibold"
          />
        ) : (
          <span className="flex shrink-0 items-center gap-2">
            <Money
              amount={receivable.amount}
              currency={receivable.currency}
              sign="never"
              className="money text-muted-foreground text-sm"
            />
            <Badge tone={receivable.status === "settled" ? "pos" : "neutral"}>
              {receivable.status === "settled" ? "Settled" : "Written off"}
            </Badge>
          </span>
        )}
      </div>

      <div
        role="progressbar"
        aria-label={`${title}: repaid`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
        className="bg-muted h-1.5 overflow-hidden rounded-full"
      >
        <div
          className="bg-positive h-full rounded-full"
          style={{ width: `${pct}%` }}
        />
      </div>
      <p className="text-muted-foreground text-xs">
        {hasPayments ? (
          <>
            <Money
              amount={receivable.paid_amount}
              currency={receivable.currency}
              sign="never"
            />{" "}
            of{" "}
            <Money
              amount={receivable.amount}
              currency={receivable.currency}
              sign="never"
            />{" "}
            repaid
          </>
        ) : (
          "Nothing repaid yet"
        )}
        {receivable.written_off_amount > 0 && (
          <>
            {" · "}
            <Money
              amount={receivable.written_off_amount}
              currency={receivable.currency}
              sign="never"
            />{" "}
            written off
          </>
        )}
      </p>

      {(open || hasMenu) && (
        <div className="flex items-center gap-1.5">
          {open && (
            <Button
              size="sm"
              variant="outline"
              className="h-10 sm:h-8"
              onClick={() => setSettleOpen(true)}
            >
              Record repayment
            </Button>
          )}
          {hasMenu && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-10 sm:size-8"
                  aria-label={`More actions for ${title}`}
                >
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                {open && (
                  <DropdownMenuItem onSelect={() => setWriteOffOpen(true)}>
                    <Eraser className="size-4" />
                    Write off
                  </DropdownMenuItem>
                )}
                {receivable.written_off_amount > 0 && (
                  <DropdownMenuItem onSelect={() => setUndoOpen(true)}>
                    <Undo2 className="size-4" />
                    Undo write-off
                  </DropdownMenuItem>
                )}
                {hasPayments && (
                  <DropdownMenuItem onSelect={() => setHistoryOpen((v) => !v)}>
                    <History className="size-4" />
                    {historyOpen ? "Hide repayments" : "Repayments"}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      )}

      {historyOpen && hasPayments && (
        <div className="bg-muted/40 rounded-md px-3 py-1">
          <ReceivableSettlementHistory receivableId={receivable.id} />
        </div>
      )}

      {open && (
        <>
          <SettleReceivableDialog
            receivable={receivable}
            open={settleOpen}
            onOpenChange={setSettleOpen}
          />
          <WriteOffReceivableDialog
            receivable={receivable}
            open={writeOffOpen}
            onOpenChange={setWriteOffOpen}
          />
        </>
      )}

      <AlertDialog open={undoOpen} onOpenChange={setUndoOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Undo this write-off?</AlertDialogTitle>
            <AlertDialogDescription>
              {receivable.counterparty_name} will owe you the written-off amount
              again, and it will no longer count as your own spending.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void undo()}>
              Undo write-off
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/**
 * One person: who, how much they still owe (per currency), and each item with
 * its repayment progress and a single repayment action.
 */
export function PersonCard({
  group,
  onRename,
}: {
  group: PersonGroup;
  onRename: () => void;
}) {
  const since = group.oldestOpen ? receivableDate(group.oldestOpen) : null;
  return (
    <div className="bg-card flex flex-col gap-4 rounded-card border p-4">
      <div className="flex items-center gap-3">
        <PersonAvatar name={group.name} personKey={group.key} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{group.name}</p>
          <p className="text-muted-foreground truncate text-xs">
            {group.openCount > 0
              ? `${group.openCount > 1 ? `${group.openCount} open items · ` : ""}${since ? `since ${formatDate(since)}` : "open"}`
              : `${group.items.length} ${group.items.length === 1 ? "item" : "items"}`}
          </p>
        </div>
        <div className="text-right">
          {group.outstanding.length > 0 ? (
            group.outstanding.map((o) => (
              <Money
                key={o.currency}
                amount={o.cents}
                currency={o.currency}
                size="md"
                sign="never"
                className="font-figures block"
              />
            ))
          ) : (
            <span className="text-muted-foreground text-sm">Nothing owed</span>
          )}
        </div>
        <Button
          size="icon"
          variant="ghost"
          className="-mr-2 size-10 sm:size-8"
          aria-label={`Rename ${group.name}`}
          onClick={onRename}
        >
          <Pencil className="size-4" />
        </Button>
      </div>
      <div className="divide-border/50 space-y-4 divide-y [&>*+*]:pt-4">
        {group.items.map((r) => (
          <ItemRow key={r.id} receivable={r} />
        ))}
      </div>
    </div>
  );
}
