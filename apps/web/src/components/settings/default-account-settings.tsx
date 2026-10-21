"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAccounts } from "@/hooks/use-accounts";
import { useAuth } from "@/hooks/use-auth";
import {
  canBeDefaultAccount,
  resolveDefaultAccountId,
} from "@/lib/default-account";

const NONE = "none";

export function DefaultAccountSettings() {
  const { defaultAccountId, setDefaultAccount } = useAuth();
  const { data: accountsData, isLoading } = useAccounts({ page_size: 100 });
  const [saving, setSaving] = useState(false);

  const accounts = accountsData?.data ?? [];
  const eligible = accounts
    .filter(canBeDefaultAccount)
    .sort((a, b) => a.name.localeCompare(b.name));
  const current = resolveDefaultAccountId(defaultAccountId, accounts) || NONE;

  async function handleChange(value: string) {
    setSaving(true);
    try {
      await setDefaultAccount(value === NONE ? null : value);
      toast.success(
        value === NONE ? "Default account cleared" : "Default account updated"
      );
    } catch {
      // setDefaultAccount already surfaced the error
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Default account</CardTitle>
        <CardDescription>
          Preselected when you add a transaction, in the web app and the
          Telegram bot. You can still pick another account each time.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Select
          value={current}
          onValueChange={handleChange}
          disabled={isLoading || saving}
        >
          <SelectTrigger className="w-full sm:w-72" aria-label="Default account">
            <SelectValue placeholder="No default" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>No default</SelectItem>
            {eligible.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}
