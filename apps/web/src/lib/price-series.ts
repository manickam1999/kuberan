import type {
  InvestmentTransaction,
  SecurityPrice,
} from "@/types/models";

/** Ranges offered on the price charts. */
export type PriceRange = "1M" | "3M" | "6M" | "1Y" | "ALL";

export const PRICE_RANGES: readonly PriceRange[] = ["1M", "3M", "6M", "1Y", "ALL"];

const RANGE_MONTHS: Record<PriceRange, number> = {
  "1M": 1,
  "3M": 3,
  "6M": 6,
  "1Y": 12,
  ALL: 120,
};

/** UTC calendar day of a Date as `YYYY-MM-DD`. */
export function isoDay(d: Date): string {
  return d.toISOString().split("T")[0];
}

/** `from_date` / `to_date` (YYYY-MM-DD) covering `range` up to `now`. */
export function rangeDates(
  range: PriceRange,
  now: Date = new Date()
): { from: string; to: string } {
  const from = new Date(now);
  from.setUTCMonth(from.getUTCMonth() - RANGE_MONTHS[range]);
  // The backend compares against midnight UTC, so go one day past today.
  const to = new Date(now);
  to.setUTCDate(to.getUTCDate() + 1);
  return { from: isoDay(from), to: isoDay(to) };
}

/** One chart point: the price in major units and the day it was recorded. */
export interface PricePoint {
  /** Day of the price, `YYYY-MM-DD`. */
  day: string;
  /** Price in cents of the security's currency. */
  cents: number;
}

/**
 * Prices (any order) as points, oldest first, one per day: the last price of a
 * day wins.
 */
export function toPricePoints(prices: readonly SecurityPrice[]): PricePoint[] {
  const byDay = new Map<string, { at: number; cents: number }>();
  for (const p of prices) {
    const at = Date.parse(p.recorded_at);
    if (Number.isNaN(at)) continue;
    const day = isoDay(new Date(at));
    const seen = byDay.get(day);
    if (!seen || at >= seen.at) byDay.set(day, { at, cents: p.price });
  }
  return [...byDay.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([day, v]) => ({ day, cents: v.cents }));
}

/** Percentage change from the first to the last point; null with fewer than two points or a zero start. */
export function changePct(points: readonly PricePoint[]): number | null {
  if (points.length < 2) return null;
  const first = points[0].cents;
  const last = points[points.length - 1].cents;
  if (first === 0) return null;
  return ((last - first) / first) * 100;
}

/** Percentage change from the previous point to the latest one. */
export function lastChangePct(points: readonly PricePoint[]): number | null {
  if (points.length < 2) return null;
  return changePct(points.slice(-2));
}

/** Lowest and highest price of the points, in cents; null when there are none. */
export function priceRange(
  points: readonly PricePoint[]
): { low: number; high: number } | null {
  if (points.length === 0) return null;
  let low = points[0].cents;
  let high = points[0].cents;
  for (const p of points) {
    if (p.cents < low) low = p.cents;
    if (p.cents > high) high = p.cents;
  }
  return { low, high };
}

export interface TradeMarkerPoint {
  /** Day of the point the marker sits on. */
  day: string;
  tone: "pos" | "neg" | "neutral";
}

/**
 * Pins buys and sells to the chart: each trade lands on the first point on or
 * after its day, or the last point when it is newer than the series; trades
 * older than the series are left off. Dividends and splits have no price to
 * mark. A day with a buy and a sell is neutral.
 */
export function tradeMarkers(
  trades: readonly Pick<InvestmentTransaction, "type" | "date">[],
  points: readonly PricePoint[]
): TradeMarkerPoint[] {
  if (points.length === 0) return [];
  const firstDay = points[0].day;
  const byDay = new Map<string, TradeMarkerPoint["tone"]>();
  for (const t of trades) {
    if (t.type !== "buy" && t.type !== "sell") continue;
    const tradeDay = t.date.slice(0, 10);
    if (tradeDay < firstDay) continue;
    const point = points.find((p) => p.day >= tradeDay) ?? points[points.length - 1];
    const tone = t.type === "buy" ? "pos" : "neg";
    const prev = byDay.get(point.day);
    byDay.set(point.day, prev && prev !== tone ? "neutral" : tone);
  }
  return [...byDay.entries()].map(([day, tone]) => ({ day, tone }));
}

/** Share of `part` in `total` as a whole percentage (0 when the total is not positive). */
export function weightPct(part: number, total: number): number {
  return total > 0 ? Math.round((part / total) * 100) : 0;
}

/**
 * A percentage with an explicit sign and a true minus (U+2212): "+3.2%",
 * "−0.4%", "0.0%". A value that rounds to zero is unsigned.
 */
export function signedPercent(value: number, fractionDigits = 1): string {
  const abs = Math.abs(value).toFixed(fractionDigits);
  if (Number(abs) === 0) return `${abs}%`;
  return `${value < 0 ? "−" : "+"}${abs}%`;
}
