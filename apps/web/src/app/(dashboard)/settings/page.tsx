"use client";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { TelegramSettings } from "@/components/settings/telegram-settings";

export default function SettingsPage() {
  return (
    <div className="container mx-auto py-6 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Settings</h1>
        <p className="text-muted-foreground mt-1">
          Manage your account settings and preferences
        </p>
      </div>

      <Tabs defaultValue="telegram" className="w-full">
        <TabsList>
          <TabsTrigger value="telegram">Telegram Bot</TabsTrigger>
          <TabsTrigger value="profile">Profile</TabsTrigger>
        </TabsList>

        <TabsContent value="telegram" className="mt-6">
          <TelegramSettings />
        </TabsContent>

        <TabsContent value="profile" className="mt-6">
          <div className="text-center py-12 text-muted-foreground">
            Profile settings coming soon...
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
