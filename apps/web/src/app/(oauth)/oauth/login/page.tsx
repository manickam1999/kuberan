"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import { apiClient, ApiClientError } from "@/lib/api-client";
import type { OAuthLoginRequest, OAuthRedirectResponse } from "@/types/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

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
      <Card>
        <CardHeader>
          <CardTitle className="text-2xl">Sign in</CardTitle>
          <CardDescription>
            This page can only be reached from an in-progress connection request.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            Missing login challenge. Start the connection from your MCP client.
          </div>
        </CardContent>
      </Card>
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
      window.location.href = res.redirect_to;
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
      window.location.href = res.redirect_to;
    } catch (err) {
      setError(getErrorMessage(err));
      setIsSubmitting(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-2xl">Sign in</CardTitle>
        <CardDescription>
          Authorize access to your Kuberan account
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="flex flex-col gap-4">
          {error && (
            <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
              {error}
            </div>
          )}
          <div className="flex flex-col gap-2">
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
          <div className="flex flex-col gap-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              placeholder="Enter your password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={isSubmitting}
            />
          </div>
        </CardContent>
        <CardFooter className="flex flex-col gap-2 pt-2">
          <Button type="submit" className="w-full" disabled={isSubmitting}>
            {isSubmitting ? "Signing in..." : "Sign in"}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={handleCancel}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

export default function OAuthLoginPage() {
  return (
    <Suspense fallback={null}>
      <OAuthLoginForm />
    </Suspense>
  );
}
