"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { SettingsSection } from "@/components/settings/settings-section";
import { useProfile, useUpdateProfile } from "@/hooks/use-profile";
import { ApiClientError } from "@/lib/api-client";

const schema = z.object({
  first_name: z.string().trim().min(1, "Enter your first name").max(100),
  last_name: z.string().trim().min(1, "Enter your last name").max(100),
  email: z.string().trim().email("Enter a valid email address"),
  current_password: z.string(),
});

type FormValues = z.infer<typeof schema>;

/** Editable name and email. Changing the email asks for the current password. */
export function ProfileForm() {
  const { data: user, isLoading, isError, refetch } = useProfile();
  const update = useUpdateProfile();

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      first_name: "",
      last_name: "",
      email: "",
      current_password: "",
    },
  });
  const {
    register,
    handleSubmit,
    reset,
    watch,
    setError,
    formState: { errors, isDirty },
  } = form;

  useEffect(() => {
    if (user) {
      reset({
        first_name: user.first_name ?? "",
        last_name: user.last_name ?? "",
        email: user.email,
        current_password: "",
      });
    }
  }, [user, reset]);

  if (isLoading) {
    return (
      <SettingsSection title="Profile">
        <div className="grid gap-4 sm:grid-cols-2">
          <Skeleton className="h-9" />
          <Skeleton className="h-9" />
          <Skeleton className="h-9 sm:col-span-2" />
        </div>
      </SettingsSection>
    );
  }

  if (isError || !user) {
    return (
      <SettingsSection title="Profile">
        <div className="flex flex-col items-start gap-3">
          <p className="text-sm text-destructive">
            Couldn&apos;t load your profile.
          </p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Try again
          </Button>
        </div>
      </SettingsSection>
    );
  }

  const emailChanged =
    watch("email").trim().toLowerCase() !== user.email.toLowerCase();

  const onSubmit = handleSubmit(async (values) => {
    if (emailChanged && !values.current_password) {
      setError("current_password", {
        message: "Enter your current password to change your email",
      });
      return;
    }
    try {
      await update.mutateAsync({
        first_name: values.first_name,
        last_name: values.last_name,
        ...(emailChanged
          ? {
              email: values.email,
              current_password: values.current_password,
            }
          : {}),
      });
      toast.success("Profile saved");
    } catch (err) {
      if (err instanceof ApiClientError && err.code === "INVALID_CURRENT_PASSWORD") {
        setError("current_password", { message: "That password isn't right" });
      } else if (err instanceof ApiClientError && err.code === "DUPLICATE_EMAIL") {
        setError("email", { message: "That email is already in use" });
      } else if (err instanceof ApiClientError && err.code === "ACCOUNT_LOCKED") {
        setError("current_password", {
          message: "Too many attempts. Try again in a few minutes.",
        });
      } else {
        toast.error("Couldn't save your profile", {
          description: err instanceof ApiClientError ? err.message : undefined,
        });
      }
    }
  });

  return (
    <SettingsSection title="Profile">
      <form onSubmit={onSubmit} noValidate className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="profile-first-name">First name</Label>
            <Input
              id="profile-first-name"
              autoComplete="given-name"
              aria-invalid={!!errors.first_name}
              {...register("first_name")}
            />
            {errors.first_name && (
              <p role="alert" className="text-xs text-destructive">
                {errors.first_name.message}
              </p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="profile-last-name">Last name</Label>
            <Input
              id="profile-last-name"
              autoComplete="family-name"
              aria-invalid={!!errors.last_name}
              {...register("last_name")}
            />
            {errors.last_name && (
              <p role="alert" className="text-xs text-destructive">
                {errors.last_name.message}
              </p>
            )}
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="profile-email">Email</Label>
            <Input
              id="profile-email"
              type="email"
              autoComplete="email"
              aria-invalid={!!errors.email}
              {...register("email")}
            />
            {errors.email && (
              <p role="alert" className="text-xs text-destructive">
                {errors.email.message}
              </p>
            )}
          </div>
          {emailChanged && (
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="profile-current-password">
                Current password
              </Label>
              <Input
                id="profile-current-password"
                type="password"
                autoComplete="current-password"
                aria-invalid={!!errors.current_password}
                {...register("current_password")}
              />
              <p className="text-xs text-muted-foreground">
                Needed to change your email address.
              </p>
              {errors.current_password && (
                <p role="alert" className="text-xs text-destructive">
                  {errors.current_password.message}
                </p>
              )}
            </div>
          )}
        </div>
        <div className="flex justify-end">
          <Button type="submit" disabled={!isDirty || update.isPending}>
            {update.isPending ? "Saving..." : "Save"}
          </Button>
        </div>
      </form>
    </SettingsSection>
  );
}
