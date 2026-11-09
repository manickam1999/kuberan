"use client";

import { useEffect, useState, type ReactNode } from "react";
import { RingMark } from "@/components/brand/ring-mark";

function Wordmark() {
  return (
    <span className="flex items-center gap-2.5">
      <RingMark className="size-8" />
      <span className="text-lg font-semibold tracking-tight">Kuberan</span>
    </span>
  );
}

/** A decorative rising curve (no axes, no numbers): it stands in for "your wealth". */
function BrandCurve() {
  return (
    <svg
      viewBox="0 0 420 120"
      className="text-primary h-auto w-full max-w-[420px]"
      aria-hidden
      focusable="false"
    >
      <defs>
        <linearGradient id="auth-curve-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.28" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path
        d="M0 98 C40 94 60 84 95 80 S150 86 185 66 S245 58 280 42 S350 30 420 12 L420 120 L0 120 Z"
        fill="url(#auth-curve-fill)"
      />
      <path
        d="M0 98 C40 94 60 84 95 80 S150 86 185 66 S245 58 280 42 S350 30 420 12"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
      <circle cx="420" cy="12" r="4" fill="currentColor" />
    </svg>
  );
}

/**
 * Split sign-in layout (prototype `signin` / `signup`): the product promise
 * beside the form on desktop, just the wordmark above the form on phones so
 * the keyboard never covers the button. Uses theme tokens only, so it follows
 * the server-default palette and the light/dark mode.
 */
export function AuthSplit({ children }: { children: ReactNode }) {
  // The host the app is served from; unknown during SSR.
  const [host, setHost] = useState<string | null>(null);
  useEffect(() => {
    setHost(window.location.hostname);
  }, []);

  return (
    <div className="grid min-h-dvh md:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
      <aside
        className="relative hidden flex-col justify-between gap-8 overflow-hidden border-r px-12 py-9 md:flex"
        style={{
          background:
            "radial-gradient(120% 80% at 0% 100%, color-mix(in oklch, var(--primary) 14%, transparent), transparent 60%)",
        }}
      >
        <Wordmark />
        <div className="flex flex-col gap-8">
          <p className="font-display text-6xl leading-[0.98] tracking-[-0.01em] lg:text-7xl">
            Your wealth,
            <br />
            on <em className="text-primary">your</em> server.
          </p>
          <BrandCurve />
        </div>
        <div className="text-muted-foreground flex flex-col gap-0.5 text-xs">
          <b className="text-foreground flex items-center gap-2 text-sm font-medium">
            <i
              aria-hidden
              className="bg-positive size-2 rounded-full not-italic"
            />
            {host || "Self-hosted"}
          </b>
          <span>Self-hosted · your Postgres · no bank logins</span>
        </div>
      </aside>

      <main className="flex flex-col px-5 pt-7 pb-10 md:justify-center md:px-12 md:py-9">
        <div className="mb-8 md:hidden">
          <Wordmark />
        </div>
        <div className="mx-auto grid w-full max-w-[380px] gap-3.5">
          {children}
        </div>
      </main>
    </div>
  );
}
