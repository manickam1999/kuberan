"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/hooks/use-auth";
import { ApiClientError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { REPORTING_CURRENCIES } from "@/lib/currencies";
import { PasswordInput } from "@/components/auth/password-input";
import { passwordStrength } from "@/lib/password-strength";
import { cn } from "@/lib/utils";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    switch (error.code) {
      case "DUPLICATE_EMAIL":
        return "An account with this email already exists";
      default:
        return error.message || "An unexpected error occurred";
    }
  }
  return "An unexpected error occurred";
}

export default function RegisterPage() {
  const { register } = useAuth();
  const router = useRouter();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [baseCurrency, setBaseCurrency] = useState("MYR");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");

    // Client-side validation
    if (!email.trim()) {
      setError("Email is required");
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setError("Please enter a valid email address");
      return;
    }
    if (email.length > 255) {
      setError("Email must be at most 255 characters");
      return;
    }
    if (!password) {
      setError("Password is required");
      return;
    }
    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    if (password.length > 128) {
      setError("Password must be at most 128 characters");
      return;
    }
    if (firstName.length > 100) {
      setError("First name must be at most 100 characters");
      return;
    }
    if (lastName.length > 100) {
      setError("Last name must be at most 100 characters");
      return;
    }

    setIsSubmitting(true);
    try {
      await register({
        email,
        password,
        first_name: firstName || undefined,
        last_name: lastName || undefined,
        base_currency: baseCurrency,
      });
      router.replace("/");
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setIsSubmitting(false);
    }
  }

  const strength = passwordStrength(password);

  return (
    <>
      <div>
        <h1 className="font-display text-4xl leading-[1.05]">
          Create your account
        </h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Your data is stored on this server only.
        </p>
      </div>
      <form onSubmit={handleSubmit} className="grid gap-3.5" noValidate>
        {error && (
          <div
            role="alert"
            className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
          >
            {error}
          </div>
        )}
        <div className="grid grid-cols-2 gap-3">
          <div className="grid gap-2">
            <Label htmlFor="firstName">First name</Label>
            <Input
              id="firstName"
              type="text"
              placeholder="John"
              autoComplete="given-name"
              value={firstName}
              onChange={(e) => setFirstName(e.target.value)}
              disabled={isSubmitting}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="lastName">Last name</Label>
            <Input
              id="lastName"
              type="text"
              placeholder="Doe"
              autoComplete="family-name"
              value={lastName}
              onChange={(e) => setLastName(e.target.value)}
              disabled={isSubmitting}
            />
          </div>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            placeholder="you@example.com"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={isSubmitting}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="password">Password</Label>
          <PasswordInput
            id="password"
            placeholder="At least 8 characters"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={isSubmitting}
          />
          <div className="flex items-center gap-2" aria-live="polite">
            <span className="flex gap-1" aria-hidden>
              {[1, 2, 3, 4].map((i) => (
                <i
                  key={i}
                  className={cn(
                    "h-1 w-6 rounded-full",
                    i <= strength.level ? "bg-primary" : "bg-muted"
                  )}
                />
              ))}
            </span>
            <span className="text-muted-foreground text-xs">
              {strength.label}
            </span>
          </div>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="baseCurrency">Reporting currency</Label>
          <Select
            value={baseCurrency}
            onValueChange={setBaseCurrency}
            disabled={isSubmitting}
          >
            <SelectTrigger id="baseCurrency" className="w-full">
              <SelectValue placeholder="Select currency" />
            </SelectTrigger>
            <SelectContent>
              {REPORTING_CURRENCIES.map((c) => (
                <SelectItem key={c.code} value={c.code}>
                  {c.code} · {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            Totals and budgets are shown in this currency. You can add accounts
            in other currencies later, and change this in Settings until you
            record an investment trade.
          </p>
        </div>
        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Creating account..." : "Create account"}
        </Button>
        <p className="text-muted-foreground text-center text-sm">
          Already have one?{" "}
          <Link href="/login" className="text-primary hover:underline">
            Sign in
          </Link>
        </p>
      </form>
    </>
  );
}
