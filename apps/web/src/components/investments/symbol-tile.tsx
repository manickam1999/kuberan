import { assetTypeColor } from "@/lib/asset-types";
import { cn } from "@/lib/utils";

/** Rounded square with a security's ticker, tinted by its asset type (prototype `.sym-t`). */
export function SymbolTile({
  symbol,
  assetType,
  className,
}: {
  symbol: string;
  assetType: string;
  className?: string;
}) {
  const color = assetTypeColor(assetType);
  // "MGS 2033" -> "MGS"; long tickers are clipped to 4 characters.
  const text = symbol.split(" ")[0].slice(0, 4);
  return (
    <span
      aria-hidden
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-control font-mono text-[11px] font-semibold",
        className
      )}
      style={{
        backgroundColor: `color-mix(in oklch, ${color} 16%, transparent)`,
        color,
      }}
    >
      {text}
    </span>
  );
}
