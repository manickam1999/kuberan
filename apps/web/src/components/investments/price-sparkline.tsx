import { cn } from "@/lib/utils";

/**
 * A tiny trend line for a list row: no axes, no tooltip, decorative. Green when
 * the series ends at or above where it starts, red otherwise. Needs two points.
 */
export function PriceSparkline({
  values,
  width = 110,
  height = 28,
  tone = "trend",
  className,
}: {
  /** Prices, oldest first (any unit). */
  values: readonly number[];
  width?: number;
  height?: number;
  /** "trend" colours by direction; "neutral" is a quiet grey line (balances). */
  tone?: "trend" | "neutral";
  className?: string;
}) {
  if (values.length < 2) return null;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const pad = 2;
  const step = (width - pad * 2) / (values.length - 1);
  const points = values
    .map((v, i) => {
      const x = pad + i * step;
      const y = pad + (1 - (v - min) / span) * (height - pad * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  const up = values[values.length - 1] >= values[0];
  return (
    <svg
      aria-hidden
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={cn("shrink-0", className)}
    >
      <polyline
        points={points}
        fill="none"
        stroke={
          tone === "neutral"
            ? "var(--muted-foreground)"
            : up
              ? "var(--positive)"
              : "var(--negative)"
        }
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}
