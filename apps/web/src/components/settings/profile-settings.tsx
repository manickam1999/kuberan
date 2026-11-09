"use client";

import { LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DefaultAccountSettings } from "@/components/settings/default-account-settings";
import { DevicesCard } from "@/components/settings/devices-card";
import { ProfileForm } from "@/components/settings/profile-form";
import { ReportingCurrencySettings } from "@/components/settings/reporting-currency-settings";
import { useAuth } from "@/hooks/use-auth";

/** Settings > Profile and security (prototype `prefs-profile`). */
export function ProfileSettings() {
  const { logout } = useAuth();

  return (
    <div className="space-y-4">
      <ProfileForm />
      <ReportingCurrencySettings />
      <DefaultAccountSettings />
      <DevicesCard />
      <div className="flex items-center justify-between gap-3">
        <Button variant="outline" onClick={logout}>
          <LogOut className="size-4" />
          Log out
        </Button>
      </div>
    </div>
  );
}
