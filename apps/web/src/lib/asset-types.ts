import type { AssetType } from "@/types/models";

/** Singular labels: "Stock", "ETF". */
export const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  bond: "Bond",
  crypto: "Crypto",
  reit: "REIT",
  commodity: "Commodity",
};

/** Tint of a symbol tile per asset type (the shared chart palette). */
export const ASSET_TYPE_COLOR: Record<AssetType, string> = {
  stock: "var(--chart-1)",
  etf: "var(--chart-3)",
  bond: "var(--chart-4)",
  crypto: "var(--chart-2)",
  reit: "var(--chart-5)",
  commodity: "var(--chart-6)",
};

/** Label of an asset type the API may send in any case; falls back to the raw value. */
export function assetTypeLabel(type: string): string {
  return ASSET_TYPE_LABELS[type.toLowerCase() as AssetType] ?? type;
}

/** Tile colour of an asset type; muted for an unknown one. */
export function assetTypeColor(type: string): string {
  return ASSET_TYPE_COLOR[type.toLowerCase() as AssetType] ?? "var(--muted-foreground)";
}
