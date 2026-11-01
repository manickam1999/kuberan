"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { apiClient, ApiClientError } from "@/lib/api-client";
import type {
  OAuthConsentAcceptRequest,
  OAuthConsentDetails,
  OAuthRedirectResponse,
} from "@/types/api";
import { isProtocolScope, isWriteScope, scopeLabel } from "@/lib/oauth-scopes";
import { describeRedirect, followHydraRedirect } from "../redirect";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

// Consent details as the API returns them. redirect_scheme/redirect_host
// describe where this authorization sends its code (taken from the request
// itself, not the client's first registered URI); client_verified is true only
// for the operator's pinned client; every other client registered itself, so
// its name is unverified.
type ConsentDetails = OAuthConsentDetails & {
  redirect_scheme?: string;
  redirect_host?: string;
  client_verified?: boolean;
};

function errorMessage(err: unknown): string {
  if (err instanceof ApiClientError) return err.message;
  if (err instanceof Error && err.message) return err.message;
  return "An unexpected error occurred";
}

// Read scopes start ticked; write scopes only when the user approved them for
// this client before (plan 022: write access is always an explicit opt-in).
function initialSelection(details: ConsentDetails): Set<string> {
  const previous = new Set(details.previously_approved ?? []);
  return new Set(
    (details.requested_scopes ?? []).filter(
      (scope) =>
        !isProtocolScope(scope) && (!isWriteScope(scope) || previous.has(scope))
    )
  );
}

function LabeledCheckbox({
  id,
  label,
  checked,
  disabled,
  muted = false,
  onChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled: boolean;
  muted?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onChange(v === true)}
        disabled={disabled}
      />
      <Label
        htmlFor={id}
        className={muted ? "text-muted-foreground font-normal" : "font-normal"}
      >
        {label}
      </Label>
    </div>
  );
}

function ScopeCheckbox({
  scope,
  checked,
  disabled,
  onChange,
}: {
  scope: string;
  checked: boolean;
  disabled: boolean;
  onChange: (scope: string, checked: boolean) => void;
}) {
  return (
    <LabeledCheckbox
      id={`scope-${scope.replace(/[^a-z0-9]/gi, "-")}`}
      label={scopeLabel(scope)}
      checked={checked}
      disabled={disabled}
      muted
      onChange={(v) => onChange(scope, v)}
    />
  );
}

function OAuthConsent() {
  const searchParams = useSearchParams();
  const consentChallenge = searchParams.get("consent_challenge") ?? "";

  const [details, setDetails] = useState<ConsentDetails | null>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [rememberClient, setRememberClient] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
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
        const res = await apiClient.get<ConsentDetails>(
          "/api/v1/oauth/consent",
          { consent_challenge: consentChallenge }
        );
        if (cancelled) return;
        // Trusted client: Hydra accepted server-side, just follow the redirect.
        if (res.redirect_to) {
          followHydraRedirect(res.redirect_to);
          return;
        }
        setDetails(res);
        setSelected(initialSelection(res));
      } catch (err) {
        if (cancelled) return;
        setError(errorMessage(err));
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
        granted_scopes: Array.from(selected),
      };
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/consent/accept",
        body
      );
      followHydraRedirect(res.redirect_to);
    } catch (err) {
      setError(errorMessage(err));
      setIsSubmitting(false);
    }
  }

  function toggleScope(scope: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) next.add(scope);
      else next.delete(scope);
      return next;
    });
  }

  async function handleDeny() {
    setError("");
    setIsSubmitting(true);
    try {
      const res = await apiClient.post<OAuthRedirectResponse>(
        "/api/v1/oauth/consent/reject",
        { consent_challenge: consentChallenge }
      );
      followHydraRedirect(res.redirect_to);
    } catch (err) {
      setError(errorMessage(err));
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
  const verified = details?.client_verified === true;
  const redirect = describeRedirect(details?.redirect_scheme, details?.redirect_host);
  const scopes = details?.requested_scopes ?? [];
  const readScopes = scopes.filter((s) => !isProtocolScope(s) && !isWriteScope(s));
  const writeScopes = scopes.filter(isWriteScope);
  const protocolScopes = scopes.filter(isProtocolScope);
  const grantsWrite = writeScopes.some((s) => selected.has(s));
  // An app that asked for permissions but would get none can't do anything;
  // Deny says that more clearly.
  const nothingSelected =
    readScopes.length + writeScopes.length > 0 && selected.size === 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-2xl">Authorize access</CardTitle>
        <CardDescription>
          An application calling itself{" "}
          <span className="text-foreground font-medium">{clientName}</span>
          {verified ? " (verified)" : ""} wants to connect to your Kuberan
          account.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}
        {!verified && (
          <p className="text-muted-foreground text-xs">
            Unverified: this app registered itself, so its name is not checked.
            Judge it by where it sends you back to.
          </p>
        )}
        <div className="flex flex-col gap-1">
          <p className="text-muted-foreground text-sm">
            Sends you back to{" "}
            <span className="text-foreground font-mono break-all">
              {redirect?.label ?? "an unknown address"}
            </span>
          </p>
          {redirect?.note && (
            <p className="text-muted-foreground text-xs">{redirect.note}</p>
          )}
          {!redirect && (
            <p className="text-muted-foreground text-xs">
              Deny unless you are sure you started this connection.
            </p>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium">This will allow it to:</p>
          {readScopes.map((scope) => (
            <ScopeCheckbox
              key={scope}
              scope={scope}
              checked={selected.has(scope)}
              disabled={isSubmitting}
              onChange={toggleScope}
            />
          ))}
          {protocolScopes.length > 0 && (
            <ul className="flex flex-col gap-1">
              {Array.from(new Set(protocolScopes.map(scopeLabel))).map((label) => (
                <li key={label} className="text-muted-foreground text-sm">
                  • {label}
                </li>
              ))}
            </ul>
          )}
        </div>
        {writeScopes.length > 0 && (
          <div className="border-warning/40 flex flex-col gap-2 rounded-md border p-3">
            <p className="text-sm font-medium">Allow changes</p>
            <p className="text-muted-foreground text-xs">
              Off unless you tick them. {clientName} will be able to change your
              financial records. You can disconnect it in Settings at any time.
            </p>
            {writeScopes.map((scope) => (
              <ScopeCheckbox
                key={scope}
                scope={scope}
                checked={selected.has(scope)}
                disabled={isSubmitting}
                onChange={toggleScope}
              />
            ))}
          </div>
        )}
        <LabeledCheckbox
          id="remember-client"
          label="Remember this application (skip this screen next time)"
          checked={rememberClient}
          disabled={isSubmitting}
          onChange={setRememberClient}
        />
        {nothingSelected && (
          <p className="text-muted-foreground text-xs">
            Tick at least one permission, or choose Deny.
          </p>
        )}
      </CardContent>
      <CardFooter className="flex flex-col gap-2 pt-2">
        <Button
          type="button"
          className="w-full"
          onClick={handleAccept}
          disabled={isSubmitting || nothingSelected}
        >
          {isSubmitting
            ? "Authorizing..."
            : grantsWrite
              ? "Authorize, including changes"
              : "Authorize"}
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
