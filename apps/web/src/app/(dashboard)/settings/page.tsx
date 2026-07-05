"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { TelegramSettings } from "@/components/settings/telegram-settings";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { useProfile } from "@/hooks/use-profile";
import { formatDateTime } from "@/lib/format";

function getInitials(first?: string, last?: string) {
  return ((first?.[0] ?? "") + (last?.[0] ?? "")).toUpperCase() || "U";
}

function ProfileTab() {
  const { data: user, isLoading } = useProfile();

  if (isLoading) {
    return (
      <Card>
        <CardContent className="flex items-center gap-4 py-6">
          <Skeleton className="size-16 rounded-full" />
          <div className="space-y-2">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-52" />
          </div>
        </CardContent>
      </Card>
    );
  }

  if (!user) return null;

  const fullName =
    [user.first_name, user.last_name].filter(Boolean).join(" ") || user.email;

  const rows: { label: string; value: string }[] = [
    { label: "First name", value: user.first_name || "—" },
    { label: "Last name", value: user.last_name || "—" },
    { label: "Email", value: user.email },
    {
      label: "Status",
      value: user.is_active === false ? "Inactive" : "Active",
    },
    {
      label: "Last login",
      value: user.last_login_at ? formatDateTime(user.last_login_at) : "—",
    },
  ];

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex items-center gap-4">
          <Avatar className="size-16 ring-2 ring-primary/25">
            <AvatarFallback className="bg-primary/15 text-lg font-semibold text-primary">
              {getInitials(user.first_name, user.last_name)}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <p className="truncate text-lg font-semibold">{fullName}</p>
            <p className="truncate text-sm text-muted-foreground">
              {user.email}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Account details</CardTitle>
        </CardHeader>
        <CardContent className="divide-y divide-border/60">
          {rows.map((row) => (
            <div
              key={row.label}
              className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0"
            >
              <span className="text-sm text-muted-foreground">{row.label}</span>
              <span className="truncate text-sm font-medium">{row.value}</span>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

export default function SettingsPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Manage your account and integrations
        </p>
      </div>

      <Tabs defaultValue="profile" className="w-full">
        <TabsList>
          <TabsTrigger value="profile">Profile</TabsTrigger>
          <TabsTrigger value="telegram">Telegram bot</TabsTrigger>
        </TabsList>

        <TabsContent value="profile" className="mt-5">
          <ProfileTab />
        </TabsContent>

        <TabsContent value="telegram" className="mt-5">
          <TelegramSettings />
        </TabsContent>
      </Tabs>
    </div>
  );
}
