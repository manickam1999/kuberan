"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, ChevronDown, Copy, RefreshCw, Unplug } from "lucide-react";
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
import { Money } from "@/components/ui/money";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { SettingsSection } from "@/components/settings/settings-section";
import {
  useAllAppActivity,
  useAppActivity,
  useConnectedApps,
  useDisconnectApp,
} from "@/hooks/use-oauth-clients";
import { ApiClientError } from "@/lib/api-client";
import { formatDateTime } from "@/lib/format";
import { getMcpConnectorUrl } from "@/lib/mcp-url";
import {
  isProtocolScope,
  isWriteScope,
  scopeLabel,
} from "@/lib/oauth-scopes";
import { formatRelativeTime } from "@/lib/relative-time";
import type { AppActivityItem, ConnectedApp } from "@/types/api";

const DISCONNECTED_NOTE =
  "Disconnected. Requests already under way may finish within a minute.";

function appName(app: ConnectedApp): string {
  return app.client_name || app.client_id;
}

function ActivityList({
  items,
  showApp,
}: {
  items: (AppActivityItem & { client_name?: string })[];
  showApp?: boolean;
}) {
  return (
    <ul className="divide-y divide-border/60">
      {items.map((item) => (
        <li
          key={item.id}
          className="flex items-start justify-between gap-3 py-2.5 first:pt-0 last:pb-0"
        >
          <div className="min-w-0">
            {/* The summary is server text: rendered as text, never as HTML. */}
            <p className="break-words text-sm">{item.summary}</p>
            {showApp && item.client_name && (
              <p className="text-xs text-muted-foreground">
                via {item.client_name}
              </p>
            )}
          </div>
          <time
            dateTime={item.created_at}
            title={formatDateTime(item.created_at)}
            className="shrink-0 text-xs text-muted-foreground"
          >
            {formatRelativeTime(item.created_at)}
          </time>
        </li>
      ))}
    </ul>
  );
}

function AppActivity({ clientId }: { clientId: string }) {
  const { data, isLoading, isError } = useAppActivity(clientId, {
    page_size: 10,
  });
  if (isLoading) return <Skeleton className="h-10" />;
  if (isError) {
    return (
      <p className="text-sm text-destructive">Couldn&apos;t load its changes.</p>
    );
  }
  if (!data || data.data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        This app hasn&apos;t changed anything.
      </p>
    );
  }
  return <ActivityList items={data.data} />;
}

function WriteLimits({ app }: { app: ConnectedApp }) {
  const limits = app.write_limits;
  if (!limits) return null;
  const parts: React.ReactNode[] = [];
  if (limits.max_amount > 0) {
    parts.push(
      <span key="max">
        up to{" "}
        <Money amount={limits.max_amount} currency={limits.currency} /> each
      </span>
    );
  }
  if (limits.per_minute > 0) {
    parts.push(<span key="min">{limits.per_minute} per minute</span>);
  }
  if (limits.per_day > 0) {
    parts.push(<span key="day">{limits.per_day} per day</span>);
  }
  if (parts.length === 0) return null;
  return (
    <span className="text-muted-foreground">
      {" · "}
      {parts.map((p, i) => (
        <span key={i}>
          {i > 0 && ", "}
          {p}
        </span>
      ))}
    </span>
  );
}

function ConnectedAppCard({
  app,
  onDisconnect,
}: {
  app: ConnectedApp;
  onDisconnect: (app: ConnectedApp) => void;
}) {
  const [showChanges, setShowChanges] = useState(false);
  const canWrite = app.scopes.some(isWriteScope);
  const see = app.scopes.filter((s) => !isProtocolScope(s) && !isWriteScope(s));
  const change = app.scopes.filter(isWriteScope);
  const name = appName(app);

  const meta = [
    app.client_host,
    app.created_at && `connected ${formatDateTime(app.created_at)}`,
    app.last_used_at
      ? `used ${formatRelativeTime(app.last_used_at)}`
      : "not used yet",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="py-4 first:pt-0 last:pb-0">
      <div className="flex items-start gap-3">
        <Tile size="xl" tone={canWrite ? "accent" : "neutral"} aria-hidden>
          <span className="font-display text-lg">
            {name.charAt(0).toUpperCase()}
          </span>
        </Tile>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium">{name}</p>
            {app.legacy ? (
              <Badge tone="warn">Reconnect to choose permissions</Badge>
            ) : (
              <Badge tone={canWrite ? "accent" : "neutral"}>
                {canWrite ? "Read & change" : "Read only"}
              </Badge>
            )}
          </div>
          <p className="break-words text-xs text-muted-foreground">{meta}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="shrink-0"
          aria-label={`Disconnect ${name}`}
          onClick={() => onDisconnect(app)}
        >
          <Unplug className="size-4" />
          <span className="hidden sm:inline">Disconnect</span>
        </Button>
      </div>

      {app.legacy ? (
        <p className="mt-3 text-sm text-muted-foreground sm:pl-[58px]">
          Connected before per-app permissions. Disconnect it and connect it
          again to choose what it can see and change.
        </p>
      ) : (
        <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2 sm:pl-[58px]">
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Can see
            </p>
            <p>
              {see.length > 0
                ? see.map((s) => scopeLabel(s).replace(/^View your /, "")).join(", ")
                : "Nothing"}
            </p>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Can change
            </p>
            <p>
              {change.length > 0 ? (
                <>
                  {change.map((s) => scopeLabel(s)).join(", ")}
                  <WriteLimits app={app} />
                </>
              ) : (
                "Nothing"
              )}
            </p>
          </div>
        </div>
      )}

      {canWrite && (
        <div className="mt-3 sm:pl-[58px]">
          <Button
            variant="ghost"
            size="sm"
            className="-ml-2.5"
            aria-expanded={showChanges}
            onClick={() => setShowChanges((v) => !v)}
          >
            <ChevronDown
              className={`size-4 transition-transform ${showChanges ? "rotate-180" : ""}`}
            />
            {showChanges ? "Hide its changes" : "Show its changes"}
          </Button>
          {showChanges && (
            <div className="mt-2">
              <AppActivity clientId={app.client_id} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function AppsList({
  onDisconnect,
  onDisconnectAll,
}: {
  onDisconnect: (app: ConnectedApp) => void;
  onDisconnectAll: () => void;
}) {
  const { data: apps, isLoading, isError, refetch, isRefetching } =
    useConnectedApps();

  if (isLoading) {
    return (
      <div className="flex items-center gap-3">
        <Skeleton className="size-[46px] rounded-[13px]" />
        <div className="space-y-2">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-3 w-64" />
        </div>
      </div>
    );
  }

  // Never show "no apps" when the list could not be loaded: this is the page
  // people use to revoke access.
  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-destructive">
          Couldn&apos;t load your connected apps.
        </p>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void refetch()}
          disabled={isRefetching}
        >
          <RefreshCw className="size-4" />
          Try again
        </Button>
      </div>
    );
  }

  if (!apps || apps.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No apps are connected. Follow the steps below to connect an assistant.
      </p>
    );
  }

  return (
    <div>
      <div className="divide-y divide-border/60">
        {apps.map((app) => (
          <ConnectedAppCard
            key={app.client_id}
            app={app}
            onDisconnect={onDisconnect}
          />
        ))}
      </div>
      {apps.length > 1 && (
        <div className="mt-4 border-t border-border/60 pt-4">
          <Button variant="ghost" className="text-destructive" onClick={onDisconnectAll}>
            <Unplug className="size-4" />
            Disconnect all apps
          </Button>
        </div>
      )}
    </div>
  );
}

function RecentChanges() {
  const { data, isLoading, isError, refetch } = useAllAppActivity({
    page_size: 8,
  });

  return (
    <SettingsSection
      title="Recent changes by apps"
      description="Every change a connected app made to your data, newest first."
    >
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      ) : isError ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-destructive">
            Couldn&apos;t load recent changes.
          </p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            <RefreshCw className="size-4" />
            Try again
          </Button>
        </div>
      ) : !data || data.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No app has changed anything yet.
        </p>
      ) : (
        <ActivityList items={data.data} showApp />
      )}
    </SettingsSection>
  );
}

function ConnectAssistant() {
  // Resolved after mount: the dev fallback reads window.location, which the
  // server render doesn't have.
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => setUrl(getMcpConnectorUrl()), []);
  const [copied, setCopied] = useState(false);

  async function copy() {
    if (!url) return;
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Couldn't copy the address");
    }
  }

  const steps: { title: string; body: React.ReactNode }[] = [
    {
      title: "Copy this server's address",
      body: url ? (
        <span className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1.5 text-xs">
            {url}
          </code>
          <Button size="sm" variant="outline" onClick={() => void copy()}>
            {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </span>
      ) : (
        <span>
          The address depends on how this server is set up. See{" "}
          <code className="rounded bg-muted px-1">docs/mcp-oauth.md</code> in
          the repository.
        </span>
      ),
    },
    {
      title: "Add it to your assistant",
      body: (
        <>
          In Claude: Settings, Connectors, Add custom connector. In Claude
          Code:{" "}
          <code className="rounded bg-muted px-1">
            claude mcp add --transport http kuberan &lt;address&gt;
          </code>
          . Other MCP apps have a similar setting.
        </>
      ),
    },
    {
      title: "Sign in and choose what it can do",
      body: "You'll land on Kuberan's sign-in and consent pages. Reading is the default; making changes is a separate opt-in you can revoke here.",
    },
  ];

  return (
    <SettingsSection
      title="Connect an assistant"
      description="Kuberan doesn't run an AI of its own. Your assistant connects to this server directly, and only what you allow is sent to its provider."
    >
      <ol className="space-y-4">
        {steps.map((s, i) => (
          <li key={s.title} className="flex gap-3">
            <span className="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-xs font-medium">
              {i + 1}
            </span>
            <div className="min-w-0 flex-1 space-y-1.5 text-sm">
              <p className="font-medium">{s.title}</p>
              <div className="text-muted-foreground">{s.body}</div>
            </div>
          </li>
        ))}
      </ol>
    </SettingsSection>
  );
}

export function ConnectedAppsSettings() {
  const disconnect = useDisconnectApp();
  const { data: apps } = useConnectedApps();
  const [pending, setPending] = useState<ConnectedApp | null>(null);
  const [confirmAll, setConfirmAll] = useState(false);
  const [busyAll, setBusyAll] = useState(false);

  async function confirmDisconnect() {
    if (!pending) return;
    try {
      await disconnect.mutateAsync(pending.client_id);
      toast.success(DISCONNECTED_NOTE);
      setPending(null);
    } catch (err) {
      // Keep the dialog open so the user can retry.
      toast.error("Failed to disconnect", {
        description:
          err instanceof ApiClientError ? err.message : "Please try again",
      });
    }
  }

  async function disconnectAll() {
    setBusyAll(true);
    try {
      for (const app of apps ?? []) {
        await disconnect.mutateAsync(app.client_id);
      }
      toast.success(DISCONNECTED_NOTE);
      setConfirmAll(false);
    } catch (err) {
      toast.error("Couldn't disconnect every app", {
        description:
          err instanceof ApiClientError ? err.message : "Please try again",
      });
    } finally {
      setBusyAll(false);
    }
  }

  return (
    <div className="space-y-4">
      <SettingsSection
        title="Connected apps"
        description="Assistants such as Claude that you've connected to Kuberan."
      >
        <AppsList
          onDisconnect={setPending}
          onDisconnectAll={() => setConfirmAll(true)}
        />
      </SettingsSection>

      <RecentChanges />
      <ConnectAssistant />

      <AlertDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open && !disconnect.isPending) setPending(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Disconnect {pending ? appName(pending) : ""}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              It will no longer be able to read or change your data, and you
              will see the consent screen if you connect it again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disconnect.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={disconnect.isPending}
              onClick={(e) => {
                // Radix closes the dialog on click; keep it open until the
                // request settles so a failure can be retried.
                e.preventDefault();
                void confirmDisconnect();
              }}
            >
              {disconnect.isPending ? "Disconnecting..." : "Disconnect"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={confirmAll}
        onOpenChange={(open) => {
          if (!open && !busyAll) setConfirmAll(false);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Disconnect all apps?</AlertDialogTitle>
            <AlertDialogDescription>
              Every connected app loses access to your data. Each one shows the
              consent screen again if you reconnect it.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busyAll}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busyAll}
              onClick={(e) => {
                e.preventDefault();
                void disconnectAll();
              }}
            >
              {busyAll ? "Disconnecting..." : "Disconnect all"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
