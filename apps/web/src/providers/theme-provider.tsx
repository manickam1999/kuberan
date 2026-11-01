"use client";

import { ThemeProvider as NextThemesProvider } from "next-themes";
import type { ReactNode } from "react";

export function ThemeProvider({
  children,
  nonce,
}: {
  children: ReactNode;
  /** CSP nonce for next-themes' inline pre-paint script. */
  nonce?: string;
}) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem
      disableTransitionOnChange
      nonce={nonce}
    >
      {children}
    </NextThemesProvider>
  );
}
