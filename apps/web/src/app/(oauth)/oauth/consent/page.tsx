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
import { PolarisMark } from "@/components/brand/polaris-mark";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tile } from "@/components/ui/tile";

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

/** Centred single-column frame: the consent step is one card on a phone screen. */
function ConsentFrame({ children }: { children: React.ReactNode }) {
  return (
    <main className="mx-auto grid min-h-dvh w-full max-w-[460px] content-center gap-3 px-5 py-5 sm:gap-3.5 sm:py-8">
      {children}
    </main>
  );
}

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
      <ConsentFrame>
        <div aria-busy="true" aria-label="Loading request details" className="grid gap-3.5">
          <div className="flex items-center gap-2.5">
            <Skeleton className="size-[42px] rounded-xl" />
            <Skeleton className="h-1 w-8" />
            <Skeleton className="size-[42px] rounded-full" />
          </div>
          <Skeleton className="h-9 w-4/5" />
          <Skeleton className="h-16 w-full rounded-xl" />
          <Skeleton className="h-36 w-full rounded-xl" />
          <Skeleton className="h-16 w-full rounded-xl" />
        </div>
      </ConsentFrame>
    );
  }

  if (error && !details) {
    return (
      <ConsentFrame>
        <h1 className="font-display text-4xl leading-[1.05]">
          Authorize access
        </h1>
        <div
          role="alert"
          className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
        >
          {error}
        </div>
      </ConsentFrame>
    );
  }

  const clientName =
    details?.client?.client_name || details?.client?.client_id || "An application";
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
    <ConsentFrame>
      <div className="flex items-center gap-2.5" aria-hidden>
        <Tile size="xl" tone="accent" className="font-display text-xl">
          {clientName.charAt(0).toUpperCase()}
        </Tile>
        <span className="bg-border h-px w-8" />
        <PolarisMark className="size-[42px]" />
      </div>
      <div>
        <h1 className="font-display text-3xl sm:text-4xl leading-[1.05]">
          {clientName} wants access to your Kuberan
        </h1>
      </div>

      {error && (
        <div
          role="alert"
          className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
        >
          {error}
        </div>
      )}

      <div className="bg-card flex flex-col gap-1 rounded-xl border p-3.5 text-sm">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <b className="font-medium">{clientName}</b>
          {verified ? (
            <Badge tone="pos">Verified</Badge>
          ) : (
            <Badge tone="warn">Unverified</Badge>
          )}
        </p>
        <p className="text-muted-foreground">
          Sends you back to{" "}
          <span className="text-foreground font-mono break-all">
            {redirect?.label ?? "an unknown address"}
          </span>
        </p>
        {redirect?.note && (
          <p className="text-muted-foreground text-xs">{redirect.note}</p>
        )}
        {!verified && (
          <p className="text-muted-foreground text-xs">
            This app registered itself, so its name is not checked. Judge it by
            where it sends you back to.
          </p>
        )}
        {!redirect && (
          <p className="text-muted-foreground text-xs">
            Deny unless you are sure you started this connection.
          </p>
        )}
      </div>

      <fieldset className="bg-background flex flex-col gap-1 rounded-xl border p-3.5">
        <legend className="sr-only">Read access</legend>
        <span className="text-muted-foreground mb-1 text-xs font-medium tracking-wide uppercase">
          Read
        </span>
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
          <ul className="mt-1 flex flex-col gap-1">
            {Array.from(new Set(protocolScopes.map(scopeLabel))).map((label) => (
              <li key={label} className="text-muted-foreground text-sm">
                • {label}
              </li>
            ))}
          </ul>
        )}
      </fieldset>

      {writeScopes.length > 0 && (
        <fieldset className="border-warning/40 bg-warning-muted/40 flex flex-col gap-1 rounded-xl border p-3.5">
          <legend className="sr-only">Changes</legend>
          <span className="text-sm font-medium">Also allow changes</span>
          <p className="text-muted-foreground mb-1 text-xs">
            Off unless you tick them. {clientName} will be able to change your
            financial records. Each change is capped in size, rate-limited and
            logged in Settings → Connected apps. You can disconnect it there at
            any time.
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
        </fieldset>
      )}

      <div className="flex items-start gap-3">
        <Switch
          id="remember-client"
          checked={rememberClient}
          onCheckedChange={setRememberClient}
          disabled={isSubmitting}
          className="mt-0.5"
        />
        <Label
          htmlFor="remember-client"
          className="flex-col items-start gap-0.5 font-normal"
        >
          <span className="text-sm">Remember this app</span>
          <span className="text-muted-foreground text-xs">
            Skip this screen next time. You can disconnect any time.
          </span>
        </Label>
      </div>

      {nothingSelected && (
        <p className="text-muted-foreground text-xs">
          Tick at least one permission, or choose Deny.
        </p>
      )}

      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button
          type="button"
          variant="ghost"
          onClick={handleDeny}
          disabled={isSubmitting}
        >
          Deny
        </Button>
        <Button
          type="button"
          onClick={handleAccept}
          disabled={isSubmitting || nothingSelected}
        >
          {isSubmitting
            ? "Authorizing..."
            : grantsWrite
              ? "Allow read access and changes"
              : "Allow read access"}
        </Button>
      </div>
    </ConsentFrame>
  );
}

export default function OAuthConsentPage() {
  return (
    <Suspense fallback={null}>
      <OAuthConsent />
    </Suspense>
  );
}
