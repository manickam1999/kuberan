"use client";

import { useState, useEffect } from "react";
import { toast } from "sonner";
import {
  Copy,
  Check,
  ExternalLink,
  RefreshCw,
  Send,
  Unlink,
} from "lucide-react";
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
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { SettingsSection } from "@/components/settings/settings-section";
import { TelegramSetupGuide } from "@/components/settings/telegram-setup-guide";
import {
  useTelegramLink,
  useGenerateLinkCode,
  useUnlinkTelegram,
} from "@/hooks/use-telegram";
import { ApiClientError } from "@/lib/api-client";
import { formatDate } from "@/lib/format";
import { formatRelativeTime } from "@/lib/relative-time";

const BOT_USERNAME = process.env.NEXT_PUBLIC_BOT_USERNAME ?? "KuberanFinBot";

/** Commands the bot really handles (apps/bot/main.py). */
const COMMANDS: { cmd: string; text: string }[] = [
  { cmd: "/expense 12.50 lunch", text: "Record an expense" },
  { cmd: "/income 3000 salary", text: "Record income" },
  { cmd: "/balance", text: "Account balances" },
  { cmd: "/summary", text: "This month's summary" },
  { cmd: "/budgets", text: "Budget status" },
  { cmd: "/owed", text: "Who owes you" },
  { cmd: "/accounts", text: "List your accounts" },
  { cmd: "/categories", text: "List your categories" },
];

function SampleChat() {
  const bubble =
    "max-w-[85%] rounded-2xl px-3 py-2 text-sm leading-snug";
  return (
    <div
      className="space-y-2 rounded-xl border bg-muted/40 p-3"
      aria-label="Example conversation"
      role="img"
    >
      <div className="flex justify-end">
        <p className={`${bubble} rounded-br-sm bg-primary text-primary-foreground`}>
          /expense 12.50 lunch
        </p>
      </div>
      <div className="flex">
        <p className={`${bubble} rounded-bl-sm border bg-card`}>
          Pick a category for your 12.50 lunch.
        </p>
      </div>
      <div className="flex justify-end">
        <p className={`${bubble} rounded-br-sm bg-primary text-primary-foreground`}>
          /balance
        </p>
      </div>
      <div className="flex">
        <p className={`${bubble} rounded-bl-sm border bg-card`}>
          Your account balances, one line per account.
        </p>
      </div>
    </div>
  );
}

function CommandsCard() {
  return (
    <SettingsSection
      title="Things you can send"
      description="Every command the bot understands."
    >
      <div className="grid gap-x-6 gap-y-2.5 sm:grid-cols-2">
        {COMMANDS.map((c) => (
          <div key={c.cmd} className="flex flex-col gap-0.5">
            <code className="w-fit rounded bg-muted px-1.5 py-0.5 text-xs">
              {c.cmd}
            </code>
            <span className="text-xs text-muted-foreground">{c.text}</span>
          </div>
        ))}
      </div>
    </SettingsSection>
  );
}

export function TelegramSettings() {
  const { data, isLoading, isError, error, refetch } = useTelegramLink();
  const generateCode = useGenerateLinkCode();
  const unlinkMutation = useUnlinkTelegram();

  const [linkCode, setLinkCode] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [showGuide, setShowGuide] = useState(false);
  const [confirmUnlink, setConfirmUnlink] = useState(false);

  const link = data?.link;
  const isLinked = !!link && link.telegram_user_id > 0 && link.is_active;
  const isPending = !isLinked && linkCode !== null;

  useEffect(() => {
    if (isLinked) {
      setLinkCode(null);
    }
  }, [isLinked]);

  const handleGenerateCode = async () => {
    try {
      const result = await generateCode.mutateAsync();
      setLinkCode(result.link_code);
      setShowGuide(true);
      toast.success("Link code generated", {
        description: "Copy the code and send it to your Telegram bot with /start",
      });
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : "Please try again";
      toast.error("Failed to generate code", { description: message });
    }
  };

  const handleCopyCode = async () => {
    if (!linkCode) return;
    try {
      await navigator.clipboard.writeText(`/start ${linkCode}`);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
      toast.success("Code copied", {
        description: "Now send it to your Telegram bot",
      });
    } catch {
      toast.error("Couldn't copy the code");
    }
  };

  const handleUnlink = async () => {
    try {
      await unlinkMutation.mutateAsync();
      setLinkCode(null);
      setConfirmUnlink(false);
      toast.success("Telegram unlinked");
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : "Please try again";
      toast.error("Failed to unlink", { description: message });
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <SettingsSection title="Telegram bot">
          <div className="flex items-center gap-3">
            <Skeleton className="size-[46px] rounded-[13px]" />
            <div className="space-y-2">
              <Skeleton className="h-4 w-44" />
              <Skeleton className="h-3 w-64" />
            </div>
          </div>
          <Skeleton className="mt-4 h-32" />
        </SettingsSection>
      </div>
    );
  }

  // A fresh account has no link yet (the API may answer 404): that is the
  // "not linked" state, not an error. Only treat a failure with no data as
  // an error when it is not a plain missing link.
  const loadFailed =
    isError && !data && !(error instanceof ApiClientError && error.status === 404);

  return (
    <div className="space-y-4">
      <SettingsSection
        title="Telegram bot"
        description="Add transactions and check your money from a chat."
        action={
          isLinked ? (
            <Badge tone="pos">Linked</Badge>
          ) : isPending ? (
            <Badge tone="warn">Pending</Badge>
          ) : undefined
        }
      >
        <div className="space-y-5">
          {isLinked && link && (
            <div className="flex items-center gap-3">
              <Tile size="xl" tone="info">
                <Send />
              </Tile>
              <div className="min-w-0">
                <p className="truncate font-medium">
                  Linked to{" "}
                  {link.telegram_username
                    ? `@${link.telegram_username}`
                    : (link.telegram_first_name ?? "your Telegram account")}
                </p>
                <p className="text-xs text-muted-foreground">
                  via @{BOT_USERNAME}
                  {link.last_message_at
                    ? ` · last message ${formatRelativeTime(link.last_message_at)} (${formatDate(link.last_message_at)})`
                    : " · no messages yet"}
                  {link.message_count > 0
                    ? ` · ${link.message_count} message${link.message_count === 1 ? "" : "s"}`
                    : ""}
                </p>
              </div>
            </div>
          )}

          {!isLinked && !isPending && (
            <div className="space-y-3">
              {loadFailed ? (
                <p className="text-sm text-destructive">
                  Couldn&apos;t check your Telegram link.
                </p>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Link your Telegram account to use{" "}
                  <a
                    href={`https://t.me/${BOT_USERNAME}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-medium text-primary hover:underline"
                  >
                    @{BOT_USERNAME}
                  </a>
                  .
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                {loadFailed ? (
                  <Button variant="outline" onClick={() => void refetch()}>
                    <RefreshCw className="size-4" />
                    Try again
                  </Button>
                ) : (
                  <Button
                    onClick={handleGenerateCode}
                    disabled={generateCode.isPending}
                  >
                    {generateCode.isPending && (
                      <RefreshCw className="size-4 animate-spin" />
                    )}
                    Generate link code
                  </Button>
                )}
                <Button
                  variant="outline"
                  onClick={() => setShowGuide(!showGuide)}
                >
                  {showGuide ? "Hide" : "Show"} setup guide
                </Button>
              </div>
            </div>
          )}

          {isPending && linkCode && (
            <Alert>
              <AlertDescription className="space-y-3">
                <div>
                  <strong className="text-sm">Link your Telegram account</strong>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Open the bot with your code, or copy the command and send
                    it yourself.
                  </p>
                </div>
                <a
                  href={`https://t.me/${BOT_USERNAME}?start=${linkCode}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex w-fit items-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
                >
                  <ExternalLink className="size-4" />
                  Open @{BOT_USERNAME} in Telegram
                </a>
                <div className="flex items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded bg-muted px-3 py-2 font-mono text-sm">
                    /start {linkCode}
                  </code>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={handleCopyCode}
                    aria-label="Copy the link command"
                  >
                    {copied ? (
                      <Check className="size-4" />
                    ) : (
                      <Copy className="size-4" />
                    )}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  The code expires in 15 minutes.{" "}
                  <button
                    type="button"
                    onClick={handleGenerateCode}
                    className="text-primary hover:underline"
                  >
                    Generate a new code
                  </button>
                </p>
                <Button variant="outline" size="sm" onClick={() => refetch()}>
                  <RefreshCw className="size-4" />
                  Check status
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <SampleChat />

          {isLinked && (
            <div className="flex flex-wrap gap-2">
              <Button asChild>
                <a
                  href={`https://t.me/${BOT_USERNAME}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Send className="size-4" />
                  Open Telegram
                </a>
              </Button>
              <Button
                variant="outline"
                className="text-destructive"
                onClick={() => setConfirmUnlink(true)}
                disabled={unlinkMutation.isPending}
              >
                <Unlink className="size-4" />
                Unlink
              </Button>
            </div>
          )}
        </div>
      </SettingsSection>

      <CommandsCard />

      {showGuide && <TelegramSetupGuide />}

      <AlertDialog open={confirmUnlink} onOpenChange={setConfirmUnlink}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Unlink Telegram?</AlertDialogTitle>
            <AlertDialogDescription>
              The bot will stop answering until you link it again with a new
              code.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={unlinkMutation.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={unlinkMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                void handleUnlink();
              }}
            >
              {unlinkMutation.isPending ? "Unlinking..." : "Unlink"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
