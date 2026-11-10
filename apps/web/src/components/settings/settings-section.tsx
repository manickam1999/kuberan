import type { ReactNode } from "react";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

interface SettingsSectionProps {
  title: string;
  description?: ReactNode;
  /** Top-right of the header (a button, a count). */
  action?: ReactNode;
  children: ReactNode;
}

/** A titled card used by every Settings tab. */
export function SettingsSection({
  title,
  description,
  action,
  children,
}: SettingsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

interface SettingsRowProps {
  title: ReactNode;
  description?: ReactNode;
  /** Right-hand control (switch, button, chip). */
  children?: ReactNode;
}

/** One row of a list inside a SettingsSection. */
export function SettingsRow({ title, description, children }: SettingsRowProps) {
  return (
    <div className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0">
        <p className="text-sm font-medium">{title}</p>
        {description && (
          <p className="text-xs text-muted-foreground">{description}</p>
        )}
      </div>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  );
}
