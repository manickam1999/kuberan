"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef } from "react";
import { useAuth } from "@/hooks/use-auth";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { AppHeader } from "@/components/layout/app-header";
import {
  CommandPalette,
  type CommandPaletteRef,
} from "@/components/layout/command-palette";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { Button } from "@/components/ui/button";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { status } = useAuth();
  const router = useRouter();
  const commandPaletteRef = useRef<CommandPaletteRef>(null);

  useEffect(() => {
    if (status === "unauthenticated") {
      router.replace("/login");
    }
  }, [status, router]);

  if (status === "unauthenticated") {
    return null;
  }

  if (status === "error") {
    return (
      <div className="flex h-screen items-center justify-center p-4">
        <div className="flex max-w-sm flex-col items-center gap-3 text-center">
          <h1 className="text-lg font-semibold">Could not reach the server</h1>
          <p className="text-sm text-muted-foreground">
            Your session is kept. Check your connection and try again.
          </p>
          <Button onClick={() => window.location.reload()}>Retry</Button>
        </div>
      </div>
    );
  }

  // Render the shell and the page while the session is still loading: the
  // middleware only lets requests with the auth flag cookie through, and the
  // API client holds page queries until the session bootstrap settles, so the
  // page's queries start in parallel with it rather than after it.
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <AppHeader
          onOpenCommandPalette={() => commandPaletteRef.current?.toggle()}
        />
        <main className="flex-1 overflow-auto scrollbar-slim">
          <div className="mx-auto w-full max-w-[1600px] px-4 py-5 sm:px-6 sm:py-7 lg:px-8">
            {children}
          </div>
        </main>
      </SidebarInset>
      <CommandPalette ref={commandPaletteRef} />
    </SidebarProvider>
  );
}
