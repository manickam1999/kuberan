"use client";

import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";
import { Bell, BellRing } from "lucide-react";
import { ApiClientError } from "@/lib/api-client";
import { useUpdateAccount } from "@/hooks/use-accounts";
import { useAccountReminders, useSetReminderEnabled } from "@/hooks/use-reminders";
import { useTelegramLink } from "@/hooks/use-telegram";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { dueDayOf } from "@/lib/account-math";
import { formatDate } from "@/lib/format";
import { pickReminder, reminderSummary } from "@/lib/reminders";
import type { Account } from "@/types/models";

const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);

function ordinal(n: number): string {
  const rem100 = n % 100;
  if (rem100 >= 11 && rem100 <= 13) return `${n}th`;
  switch (n % 10) {
    case 1:
      return `${n}st`;
    case 2:
      return `${n}nd`;
    case 3:
      return `${n}rd`;
    default:
      return `${n}th`;
  }
}

/** Day-of-month picker (1 to 31). */
export function DueDaySelect({
  id,
  value,
  onChange,
  disabled,
  allowNone,
}: {
  id?: string;
  /** 0 means "no due day". */
  value: number;
  onChange: (day: number) => void;
  disabled?: boolean;
  allowNone?: boolean;
}) {
  return (
    <Select
      value={String(value)}
      onValueChange={(v) => onChange(Number(v))}
      disabled={disabled}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue placeholder="Pick a day" />
      </SelectTrigger>
      <SelectContent>
        {allowNone && <SelectItem value="0">No due day</SelectItem>}
        {DAYS.map((d) => (
          <SelectItem key={d} value={String(d)}>
            {ordinal(d)} of the month
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Sets the day of the month a card or loan falls due; this is what schedules the reminder. */
function DueDayDialog({
  account,
  open,
  onOpenChange,
}: {
  account: Account;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const update = useUpdateAccount(account.id);
  const [day, setDay] = useState(dueDayOf(account) ?? 1);
  const noun = account.type === "debt" ? "loan payment" : "card payment";

  function save() {
    update.mutate(
      { due_day: day },
      {
        onSuccess: () => {
          toast.success(`Reminder set for the ${ordinal(day)}`);
          onOpenChange(false);
        },
        onError: (err) =>
          toast.error(
            err instanceof ApiClientError ? err.message : "Couldn't set the reminder",
          ),
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Schedule a reminder</DialogTitle>
          <DialogDescription>
            Pick the day each month your {noun} is due. We&apos;ll message you
            on Telegram a few days before.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor="reminder-due-day">Due day</Label>
          <DueDaySelect
            id="reminder-due-day"
            value={day}
            onChange={setDay}
            disabled={update.isPending}
          />
        </div>
        <DialogFooter>
          <Button onClick={save} disabled={update.isPending}>
            {update.isPending ? "Saving..." : "Schedule reminder"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Reminder status for a credit card or loan: a "Schedule a reminder" button
 * until a due day is set, then the next reminder with an on/off switch. The
 * API creates and moves the reminder with the account's due day.
 */
export function ReminderControl({ account }: { account: Account }) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const hasDay = !!account.due_day;
  const { data: reminders } = useAccountReminders(hasDay ? account.id : "");
  const setEnabled = useSetReminderEnabled(account.id);
  const { data: telegram } = useTelegramLink();
  const linked =
    !!telegram?.link && telegram.link.telegram_user_id > 0 && telegram.link.is_active;
  const reminder = pickReminder(reminders ?? []);

  if (!hasDay || !reminder) {
    return (
      <>
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2 w-fit"
          onClick={() => setDialogOpen(true)}
        >
          <Bell className="size-4" />
          Schedule a reminder
        </Button>
        <DueDayDialog
          account={account}
          open={dialogOpen}
          onOpenChange={setDialogOpen}
        />
      </>
    );
  }

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <BellRing
            className={`size-4 shrink-0 ${reminder.enabled ? "text-primary" : "text-muted-foreground"}`}
          />
          <div className="min-w-0">
            <p className="text-sm font-medium">
              Reminder {reminder.enabled ? "on" : "off"}
            </p>
            <p className="text-muted-foreground text-xs">
              {reminderSummary(reminder, formatDate)}
              {" · "}
              <button
                type="button"
                className="text-primary hover:underline"
                onClick={() => setDialogOpen(true)}
              >
                Change day
              </button>
            </p>
          </div>
        </div>
        <Switch
          aria-label="Remind me on Telegram"
          checked={reminder.enabled}
          disabled={setEnabled.isPending}
          onCheckedChange={(enabled) =>
            setEnabled.mutate(
              { id: reminder.id, enabled },
              {
                onError: () => toast.error("Couldn't update the reminder"),
              },
            )
          }
        />
      </div>
      {reminder.enabled && !linked && (
        <p className="text-muted-foreground text-xs">
          Reminders arrive on Telegram.{" "}
          <Link href="/settings?tab=telegram" className="text-primary hover:underline">
            Link your account
          </Link>{" "}
          to receive them.
        </p>
      )}
      <DueDayDialog
        key={account.due_day}
        account={account}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </div>
  );
}
