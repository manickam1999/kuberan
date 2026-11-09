import type { Metadata, Viewport } from "next";
import { headers } from "next/headers";
import {
  Geist,
  Geist_Mono,
  Instrument_Serif,
  Source_Serif_4,
} from "next/font/google";
import localFont from "next/font/local";
import { ThemeProvider } from "@/providers/theme-provider";
import { QueryProvider } from "@/providers/query-provider";
import { AuthProvider } from "@/providers/auth-provider";
import {
  ChartThemeProvider,
  CHART_THEME_INIT_SCRIPT,
} from "@/providers/chart-theme-provider";
import { PALETTE_INIT_SCRIPT } from "@/lib/palette-apply";
import { AppearanceSync } from "@/providers/appearance-sync";
import { Toaster } from "@/components/ui/sonner";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

// Display face (page titles, brand) and figures face (hero / KPI amounts).
// Self-hosted at build time by next/font, so the CSP's `font-src 'self'` holds.
const instrumentSerif = Instrument_Serif({
  variable: "--font-instrument-serif",
  subsets: ["latin"],
  weight: "400",
  style: ["normal", "italic"],
  display: "swap",
});

const sourceSerif = Source_Serif_4({
  variable: "--font-source-serif",
  subsets: ["latin"],
  axes: ["opsz"],
  display: "swap",
});

// Geist Pixel (Square) — the pixel accent dither-kit uses on its own site
// (Vercel's Geist Pixel, OFL; woff2 vendored under ./fonts). Loaded globally;
// applied to headings + chart labels only when `data-chart-theme="dither"` is
// set (see globals.css). Body stays Geist, matching dither-kit.
const geistPixel = localFont({
  src: "./fonts/GeistPixel-Square.woff2",
  variable: "--font-geist-pixel-square",
  weight: "500",
  display: "swap",
  // Only the opt-in dither chart theme uses it, so don't spend a preload on
  // every page load.
  preload: false,
  fallback: ["Geist Mono", "ui-monospace", "monospace"],
});

export const metadata: Metadata = {
  title: "Kuberan",
  description: "Personal finance tracker",
  applicationName: "Kuberan",
  // apple-mobile-web-app-* metas: iOS has no install event, so these make
  // "Add to Home Screen" open full screen with a translucent status bar (the
  // header pads by the safe-area top inset).
  appleWebApp: {
    capable: true,
    title: "Kuberan",
    statusBarStyle: "black-translucent",
  },
  formatDetection: { telephone: false },
  // Next emits the standard mobile-web-app-capable; older iOS wants the
  // prefixed name too.
  other: { "apple-mobile-web-app-capable": "yes" },
};

// viewport-fit=cover lets the layout extend under the notch / home indicator;
// the header and tab bar pad by the safe-area insets. themeColor follows the
// default palette's background per colour scheme.
export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#e6e9ef" },
    { media: "(prefers-color-scheme: dark)", color: "#181825" },
  ],
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  // Per-request CSP nonce set by the middleware. Reading headers() makes every
  // page dynamic, which a nonce-based CSP requires anyway.
  const nonce = (await headers()).get("x-nonce") ?? undefined;

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply the persisted chart theme before paint to avoid a flash. */}
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: CHART_THEME_INIT_SCRIPT }}
        />
        {/* Apply the mirrored palette (cookie, then localStorage) before paint. */}
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: PALETTE_INIT_SCRIPT }}
        />
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} ${geistPixel.variable} ${instrumentSerif.variable} ${sourceSerif.variable} antialiased`}
      >
        <ThemeProvider nonce={nonce}>
          <ChartThemeProvider>
            <QueryProvider>
              <AuthProvider>
                <AppearanceSync />
                {children}
                <Toaster />
              </AuthProvider>
            </QueryProvider>
          </ChartThemeProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
