import type { MetadataRoute } from "next";

/**
 * Web app manifest (D6: installable PWA, no service worker or offline
 * support). The Polaris mark ships as 192/512 "any" PNGs, a maskable 512 and the
 * SVG. A manifest carries a single theme_color; the per-scheme colours live in
 * the `viewport` export of app/layout.tsx. The dark default is used here
 * because the splash screen is drawn from the manifest.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "Kuberan",
    short_name: "Kuberan",
    description: "Personal finance tracker",
    start_url: "/",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#181825",
    theme_color: "#181825",
    categories: ["finance"],
    icons: [
      {
        src: "/icon-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any",
      },
      {
        src: "/icon-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any",
      },
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml" },
      {
        src: "/icon-maskable-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
    shortcuts: [
      { name: "Transactions", url: "/transactions" },
      { name: "Accounts", url: "/accounts" },
    ],
  };
}
