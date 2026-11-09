"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { RingMark } from "@/components/brand/ring-mark";
import { useAuth } from "@/hooks/use-auth";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isLoading, isAuthenticated } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.replace("/");
    }
  }, [isLoading, isAuthenticated, router]);

  if (isLoading || isAuthenticated) {
    return null;
  }

  return (
    <div className="relative flex min-h-dvh items-center justify-center overflow-hidden p-4">
      {/* Ambient brand glow */}
      <div
        aria-hidden
        className="pointer-events-none absolute -top-40 left-1/2 size-[38rem] -translate-x-1/2 rounded-full opacity-60 blur-3xl"
        style={{
          background:
            "radial-gradient(closest-side, color-mix(in oklch, var(--primary) 22%, transparent), transparent)",
        }}
      />

      <div className="relative w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <RingMark className="size-12" />
          <div>
            <h1 className="font-display text-4xl leading-none">Kuberan</h1>
            <p className="text-sm text-muted-foreground">
              Your finances, in focus
            </p>
          </div>
        </div>

        {children}

        <p className="text-center text-xs text-muted-foreground">
          Self-hosted, privacy-first finance tracking
        </p>
      </div>
    </div>
  );
}
