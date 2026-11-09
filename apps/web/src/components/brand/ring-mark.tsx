import { cn } from "@/lib/utils";

/**
 * Kuberan's ring mark: a thin ring around a serif-like K, drawn in
 * `currentColor` (the accent, in the app). Placeholder until a designer
 * supplies the final mark; keep `public`-side assets (`app/icon.svg`) in
 * step with this geometry.
 */
export function RingMark({
  className,
  title,
}: {
  className?: string;
  /** Accessible name; omit when the wordmark sits beside it (decorative). */
  title?: string;
}) {
  return (
    <svg
      viewBox="0 0 32 32"
      fill="none"
      stroke="currentColor"
      className={cn("size-8 shrink-0 text-primary", className)}
      role={title ? "img" : undefined}
      aria-label={title}
      aria-hidden={title ? undefined : true}
    >
      <circle cx="16" cy="16" r="14.25" strokeWidth="1.5" />
      <path
        d="M12.5 9v14M12.5 16.2 20 9M15.6 14.4 20.6 23"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
