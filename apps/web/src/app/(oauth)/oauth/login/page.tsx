"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Plug } from "lucide-react";
import { apiClient, ApiClientError } from "@/lib/api-client";
import type { OAuthLoginRequest, OAuthRedirectResponse } from "@/types/api";
import { followHydraRedirect } from "../redirect";
import { AuthSplit } from "@/components/auth/auth-split";
import { PasswordInput } from "@/components/auth/password-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tile } from "@/components/ui/tile";

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    switch (error.code) {
      case "INVALID_CREDENTIALS":
        return "Invalid email or password";
      case "ACCOUNT_LOCKED":
        return "Account locked due to too many failed attempts. Try again later.";
      default:
        return error.message || "An unexpected error occurred";
    }
  }
  if (error instanceof Error && error.message) return error.message;
  return "An unexpected error occurred";
}

function OAuthLoginForm() {
  const searchParams = useSearchParams();
  const loginChallenge = searchParams.get("login_challenge") ?? "";

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!loginChallenge) {
    return (
      <>
        <div>
          <h1 className="font-display text-4xl leading-[1.05]">Sign in</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            This page can only be reached from an in-progress connection
            request.
          </p>
        </div>
        <div
          role="alert"
          className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
        >
          Missing login challenge. Start the connection from your MCP client.
        </div>
      </>
    );
  }

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");

    if (!email.trim()) {
      setError("Email is required");
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setError("Please enter a valid email address");
      return;
    }
    if (!password) {
      setError("Password is required");
      return;
    }

    setIsSubmitting(true);
    try {
      const body: OAuthLoginRequest = {
        login_challenge: loginChallenge,
        email,
        password,
      };
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/login",
        body
      );
      // Hand control back to Hydra, which drives the consent step next.
      followHydraRedirect(res.redirect_to);
    } catch (err) {
      setError(getErrorMessage(err));
      setIsSubmitting(false);
    }
  }

  // Cancel a login the user did not initiate. Because this page is only reachable
  // mid-flow, an unexpected sign-in prompt is a phishing signal; declining rejects
  // the Hydra challenge so the connector sees an explicit denial.
  async function handleCancel() {
    setError("");
    setIsSubmitting(true);
    try {
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/login/reject",
        { login_challenge: loginChallenge }
      );
      followHydraRedirect(res.redirect_to);
    } catch (err) {
      setError(getErrorMessage(err));
      setIsSubmitting(false);
    }
  }

  return (
    <>
      <div className="bg-card flex items-start gap-3 rounded-xl border p-3.5">
        <Tile size="lg" tone="accent" aria-hidden>
          <Plug />
        </Tile>
        <div className="min-w-0 text-sm">
          <b className="block font-medium">An app wants to connect</b>
          <span className="text-muted-foreground text-xs">
            Sign in to Kuberan to continue. You&apos;ll choose what it can do
            next.
          </span>
        </div>
      </div>
      <div>
        <h1 className="font-display text-4xl leading-[1.05]">
          Sign in to continue
        </h1>
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
            placeholder="Enter your password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={isSubmitting}
          />
        </div>
        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Signing in..." : "Sign in"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="w-full"
          onClick={handleCancel}
          disabled={isSubmitting}
        >
          Cancel and return to the app
        </Button>
        <p className="text-muted-foreground text-center text-xs">
          After 5 wrong passwords the account locks for 15 minutes.
        </p>
      </form>
    </>
  );
}

export default function OAuthLoginPage() {
  return (
    <AuthSplit>
      <Suspense fallback={null}>
        <OAuthLoginForm />
      </Suspense>
    </AuthSplit>
  );
}
