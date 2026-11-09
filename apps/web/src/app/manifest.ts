import type { MetadataRoute } from "next";

/** Web app manifest: the ring mark as standard and maskable icons. */
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Kuberan",
    short_name: "Kuberan",
    description: "Personal finance tracker",
    start_url: "/",
    display: "standalone",
    background_color: "#1e1e2e",
    theme_color: "#1e1e2e",
    icons: [
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml" },
      {
        src: "/icon-maskable-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
  };
}
