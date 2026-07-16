import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import localFont from "next/font/local";
import { ThemeProvider } from "@/providers/theme-provider";
import { QueryProvider } from "@/providers/query-provider";
import { AuthProvider } from "@/providers/auth-provider";
import {
  ChartThemeProvider,
  CHART_THEME_INIT_SCRIPT,
} from "@/providers/chart-theme-provider";
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

// Geist Pixel (Square) — the pixel accent dither-kit uses on its own site
// (Vercel's Geist Pixel, OFL; woff2 vendored under ./fonts). Loaded globally;
// applied to headings + chart labels only when `data-chart-theme="dither"` is
// set (see globals.css). Body stays Geist, matching dither-kit.
const geistPixel = localFont({
  src: "./fonts/GeistPixel-Square.woff2",
  variable: "--font-geist-pixel-square",
  weight: "500",
  display: "swap",
  fallback: ["Geist Mono", "ui-monospace", "monospace"],
});

export const metadata: Metadata = {
  title: "Kuberan",
  description: "Personal finance tracker",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Apply the persisted chart theme before paint to avoid a flash. */}
        <script dangerouslySetInnerHTML={{ __html: CHART_THEME_INIT_SCRIPT }} />
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} ${geistPixel.variable} antialiased`}
      >
        <ThemeProvider>
          <ChartThemeProvider>
            <QueryProvider>
              <AuthProvider>
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
