"use client";

import { useState } from "react";
import { Laptop, Lock, Smartphone, Tablet, HelpCircle, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tile } from "@/components/ui/tile";
import { ChangePasswordDialog } from "@/components/settings/change-password-dialog";
import { SettingsSection } from "@/components/settings/settings-section";
import {
  useRevokeOtherSessions,
  useRevokeSession,
  useSessions,
} from "@/hooks/use-sessions";
import { ApiClientError } from "@/lib/api-client";
import { parseDeviceLabel, type DeviceKind } from "@/lib/device-label";
import { formatRelativeTime } from "@/lib/relative-time";
import type { AuthSession } from "@/types/api";

const KIND_ICON: Record<DeviceKind, typeof Laptop> = {
  desktop: Laptop,
  phone: Smartphone,
  tablet: Tablet,
  unknown: HelpCircle,
};

function DeviceRow({
  session,
  onRevoke,
  revoking,
}: {
  session: AuthSession;
  onRevoke: (id: string) => void;
  revoking: boolean;
}) {
  const { label, kind } = parseDeviceLabel(session.user_agent);
  const Icon = KIND_ICON[kind];
  const seen = session.current
    ? "This device"
    : `Active ${formatRelativeTime(session.last_seen_at)}`;

  return (
    <div className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
      <Tile tone="neutral">
        <Icon />
      </Tile>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{label}</p>
        <p className="truncate text-xs text-muted-foreground">
          {seen}
          {session.ip_address ? ` · ${session.ip_address}` : ""}
        </p>
      </div>
      {session.current ? (
        <Badge tone="accent">Current</Badge>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          disabled={revoking}
          aria-label={`Sign out ${label}`}
          onClick={() => onRevoke(session.id)}
        >
          Sign out
        </Button>
      )}
    </div>
  );
}

/** Signed-in devices with revoke, "sign out everywhere else" and change password. */
export function DevicesCard() {
  const { data: sessions, isLoading, isError, refetch, isRefetching } =
    useSessions();
  const revoke = useRevokeSession();
  const revokeOthers = useRevokeOtherSessions();
  const [passwordOpen, setPasswordOpen] = useState(false);

  const others = (sessions ?? []).filter((s) => !s.current).length;

  async function handleRevoke(id: string) {
    try {
      await revoke.mutateAsync(id);
      toast.success("Device signed out");
    } catch (err) {
      toast.error("Couldn't sign that device out", {
        description: err instanceof ApiClientError ? err.message : undefined,
      });
    }
  }

  async function handleRevokeOthers() {
    try {
      const res = await revokeOthers.mutateAsync();
      toast.success(
        res.revoked_sessions === 1
          ? "1 device signed out"
          : `${res.revoked_sessions} devices signed out`
      );
    } catch (err) {
      toast.error("Couldn't sign the other devices out", {
        description: err instanceof ApiClientError ? err.message : undefined,
      });
    }
  }

  return (
    <SettingsSection
      title="Signed-in devices"
      description="Everywhere you're signed in to Kuberan right now."
    >
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </div>
      ) : isError ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-destructive">
            Couldn&apos;t load your devices.
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
      ) : (
        <div className="divide-y divide-border/60">
          {(sessions ?? []).map((s) => (
            <DeviceRow
              key={s.id}
              session={s}
              onRevoke={(id) => void handleRevoke(id)}
              revoking={revoke.isPending}
            />
          ))}
        </div>
      )}

      <div className="mt-5 flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => setPasswordOpen(true)}>
          <Lock className="size-4" />
          Change password
        </Button>
        <Button
          variant="ghost"
          disabled={others === 0 || revokeOthers.isPending}
          onClick={() => void handleRevokeOthers()}
        >
          Sign out everywhere else
        </Button>
      </div>
      <ChangePasswordDialog open={passwordOpen} onOpenChange={setPasswordOpen} />
    </SettingsSection>
  );
}
