"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useChangePassword } from "@/hooks/use-sessions";
import { ApiClientError } from "@/lib/api-client";

const schema = z
  .object({
    current_password: z.string().min(1, "Enter your current password"),
    new_password: z
      .string()
      .min(8, "Use at least 8 characters")
      .refine((v) => new TextEncoder().encode(v).length <= 72, "Use at most 72 bytes"),
    confirm: z.string(),
  })
  .refine((v) => v.new_password === v.confirm, {
    path: ["confirm"],
    message: "The passwords don't match",
  });

type FormValues = z.infer<typeof schema>;

interface ChangePasswordDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ChangePasswordDialog({
  open,
  onOpenChange,
}: ChangePasswordDialogProps) {
  const change = useChangePassword();
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { current_password: "", new_password: "", confirm: "" },
  });

  useEffect(() => {
    if (!open) reset();
  }, [open, reset]);

  const onSubmit = handleSubmit(async (values) => {
    try {
      const res = await change.mutateAsync({
        current_password: values.current_password,
        new_password: values.new_password,
      });
      toast.success("Password changed", {
        description:
          res.revoked_sessions > 0
            ? `${res.revoked_sessions} other ${res.revoked_sessions === 1 ? "device was" : "devices were"} signed out.`
            : "No other devices were signed in.",
      });
      onOpenChange(false);
    } catch (err) {
      if (err instanceof ApiClientError && err.code === "INVALID_CURRENT_PASSWORD") {
        setError("current_password", { message: "That password isn't right" });
      } else if (err instanceof ApiClientError && err.code === "ACCOUNT_LOCKED") {
        setError("current_password", {
          message: "Too many attempts. Try again in a few minutes.",
        });
      } else {
        toast.error("Couldn't change your password", {
          description: err instanceof ApiClientError ? err.message : undefined,
        });
      }
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change password</DialogTitle>
          <DialogDescription>
            Your other devices will be signed out. The Telegram bot and
            connected apps stay connected; to cut them off, unlink Telegram or{" "}
            <Link
              href="/settings?tab=connected-apps"
              className="text-primary underline-offset-2 hover:underline"
              onClick={() => onOpenChange(false)}
            >
              disconnect all apps
            </Link>
            .
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} noValidate className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="pw-current">Current password</Label>
            <Input
              id="pw-current"
              type="password"
              autoComplete="current-password"
              aria-invalid={!!errors.current_password}
              {...register("current_password")}
            />
            {errors.current_password && (
              <p role="alert" className="text-xs text-destructive">
                {errors.current_password.message}
              </p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pw-new">New password</Label>
            <Input
              id="pw-new"
              type="password"
              autoComplete="new-password"
              aria-invalid={!!errors.new_password}
              {...register("new_password")}
            />
            {errors.new_password && (
              <p role="alert" className="text-xs text-destructive">
                {errors.new_password.message}
              </p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pw-confirm">Confirm new password</Label>
            <Input
              id="pw-confirm"
              type="password"
              autoComplete="new-password"
              aria-invalid={!!errors.confirm}
              {...register("confirm")}
            />
            {errors.confirm && (
              <p role="alert" className="text-xs text-destructive">
                {errors.confirm.message}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={change.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={change.isPending}>
              {change.isPending ? "Changing..." : "Change password"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
