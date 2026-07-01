"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { apiClient, ApiClientError } from "@/lib/api-client";
import type {
  OAuthConsentAcceptRequest,
  OAuthConsentDetails,
  OAuthRedirectResponse,
} from "@/types/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

// Human-readable labels for the granular read:* scopes Hydra may request.
const SCOPE_LABELS: Record<string, string> = {
  "read:accounts": "View your accounts",
  "read:transactions": "View your transactions",
  "read:budgets": "View your budgets",
  "read:categories": "View your categories",
  "read:investments": "View your investments",
  "read:portfolio": "View your portfolio",
  "read:snapshots": "View your net-worth history",
  openid: "Verify your identity",
  offline_access: "Maintain access when you're away",
};

function scopeLabel(scope: string): string {
  return SCOPE_LABELS[scope] ?? scope;
}

// Show the host of the client's redirect target — the anti-phishing signal.
function redirectHost(redirectUris?: string[]): string | null {
  const uri = redirectUris?.[0];
  if (!uri) return null;
  try {
    return new URL(uri).host;
  } catch {
    return uri;
  }
}

function OAuthConsent() {
  const searchParams = useSearchParams();
  const consentChallenge = searchParams.get("consent_challenge") ?? "";

  const [details, setDetails] = useState<OAuthConsentDetails | null>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [rememberClient, setRememberClient] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!consentChallenge) {
      setError("Missing consent challenge. Start the connection from your MCP client.");
      setIsLoading(false);
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        const res = await apiClient.get<OAuthConsentDetails>(
          "/api/v1/oauth/consent",
          { consent_challenge: consentChallenge }
        );
        if (cancelled) return;
        // Trusted client: Hydra accepted server-side, just follow the redirect.
        if (res.redirect_to) {
          window.location.href = res.redirect_to;
          return;
        }
        setDetails(res);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiClientError
            ? err.message
            : "An unexpected error occurred"
        );
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [consentChallenge]);

  async function handleAccept() {
    setError("");
    setIsSubmitting(true);
    try {
      const body: OAuthConsentAcceptRequest = {
        consent_challenge: consentChallenge,
        remember_client: rememberClient,
      };
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/consent/accept",
        body
      );
      window.location.href = res.redirect_to;
    } catch (err) {
      setError(
        err instanceof ApiClientError
          ? err.message
          : "An unexpected error occurred"
      );
      setIsSubmitting(false);
    }
  }

  async function handleDeny() {
    setError("");
    setIsSubmitting(true);
    try {
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/consent/reject",
        { consent_challenge: consentChallenge }
      );
      window.location.href = res.redirect_to;
    } catch (err) {
      setError(
        err instanceof ApiClientError
          ? err.message
          : "An unexpected error occurred"
      );
      setIsSubmitting(false);
    }
  }

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-2xl">Authorize access</CardTitle>
          <CardDescription>Loading request details...</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  if (error && !details) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-2xl">Authorize access</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        </CardContent>
      </Card>
    );
  }

  const clientName = details?.client?.client_name || details?.client?.client_id || "An application";
  const host = redirectHost(details?.redirect_uris);
  const scopes = details?.requested_scopes ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-2xl">Authorize access</CardTitle>
        <CardDescription>
          <span className="text-foreground font-medium">{clientName}</span> wants
          to connect to your Kuberan account.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}
        {host && (
          <p className="text-muted-foreground text-sm">
            Redirects to{" "}
            <span className="text-foreground font-mono">{host}</span>
          </p>
        )}
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium">This will allow it to:</p>
          <ul className="flex flex-col gap-1">
            {scopes.map((scope) => (
              <li key={scope} className="text-muted-foreground text-sm">
                • {scopeLabel(scope)}
              </li>
            ))}
          </ul>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="border-input size-4 rounded"
            checked={rememberClient}
            onChange={(e) => setRememberClient(e.target.checked)}
            disabled={isSubmitting}
          />
          <span>Remember this application (skip this screen next time)</span>
        </label>
      </CardContent>
      <CardFooter className="flex flex-col gap-2 pt-2">
        <Button
          type="button"
          className="w-full"
          onClick={handleAccept}
          disabled={isSubmitting}
        >
          {isSubmitting ? "Authorizing..." : "Authorize"}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="w-full"
          onClick={handleDeny}
          disabled={isSubmitting}
        >
          Deny
        </Button>
      </CardFooter>
    </Card>
  );
}

export default function OAuthConsentPage() {
  return (
    <Suspense fallback={null}>
      <OAuthConsent />
    </Suspense>
  );
}
