import { cn } from "@/lib/utils";

/**
 * Kuberan's mark, "Polaris facet": a four-point star with a long northern ray
 * (Kubera guards the north), cut into four facets like a gem. Drawn in
 * `currentColor` (the accent, in the app); two facets are at half strength.
 *
 * The facets read from about 28px up. Favicons (16-32px) use the solid
 * silhouette instead: keep `app/icon.svg`, `app/favicon.ico`, the PWA icons
 * and `apps/site` in step with this geometry: `pnpm icons`
 * (`scripts/render-icons.mjs`) regenerates all of them.
 */
export function PolarisMark({
  className,
  title,
}: {
  className?: string;
  /** Accessible name; omit when the wordmark sits beside it (decorative). */
  title?: string;
}) {
  return (
    <svg
      viewBox="3.5 2.5 57 57"
      fill="currentColor"
      className={cn("size-8 shrink-0 text-primary", className)}
      role={title ? "img" : undefined}
      aria-label={title}
      aria-hidden={title ? undefined : true}
    >
      <path d="M14 31.5C28 27.5 30.4 21 32 3V31.5ZM50 31.5C36 35.5 33.6 41 32 54V31.5Z" />
      <path
        d="M32 3C33.6 21 36 27.5 50 31.5H32ZM32 54C30.4 41 28 35.5 14 31.5H32Z"
        opacity={0.5}
      />
    </svg>
  );
}
