/**
 * Pure maths behind the Record trade sheet. It mirrors the backend's
 * `holdingState.apply` (apps/api/internal/services/investment_math.go): a sell
 * removes a proportional share of the remaining cost (average cost) and the
 * realised gain is the net proceeds minus that share. All amounts are integer
 * cents in the security's currency.
 */

/** Share quantities closer than this are the same quantity (the backend's `quantityEpsilon`). */
export const QUANTITY_EPSILON = 1e-9;

/** `quantity x price` in whole cents, rounded once (the backend's `lineAmount`). */
export function lineAmount(quantity: number, pricePerUnit: number): number {
  if (!(quantity > 0) || !(pricePerUnit > 0)) return 0;
  return Math.round(quantity * pricePerUnit);
}

/** The part of `cost` removed when `quantity` of a `position` is sold (`proportionalCost`). */
export function proportionalCost(
  cost: number,
  quantity: number,
  position: number
): number {
  if (position <= 0) return 0;
  if (quantity >= position - QUANTITY_EPSILON) return cost;
  return Math.round(cost * (quantity / position));
}

export interface SellPreview {
  /** quantity x price, before the fee. */
  gross: number;
  /** Net proceeds: gross minus the fee (what the account receives). */
  proceeds: number;
  /** Cost basis removed with the shares sold (average cost). */
  costSold: number;
  /** proceeds - costSold. */
  realizedGain: number;
  /** Realised gain as a percentage of the cost sold (0 when there is no cost). */
  realizedPct: number;
  /** The sale empties the position. */
  closesPosition: boolean;
}

export interface SellPreviewInput {
  quantity: number;
  pricePerUnit: number;
  fee: number;
  /** Units held now. */
  position: number;
  /** Remaining cost basis of the position, in the security's currency. */
  costBasis: number;
}

/**
 * Preview of a sale, or `null` while the inputs cannot form one (no quantity
 * or price, or more units than held).
 */
export function previewSell(input: SellPreviewInput): SellPreview | null {
  const { quantity, pricePerUnit, fee, position, costBasis } = input;
  if (!(quantity > 0) || !(pricePerUnit > 0)) return null;
  if (quantity > position + QUANTITY_EPSILON) return null;
  const gross = lineAmount(quantity, pricePerUnit);
  const proceeds = gross - Math.max(fee, 0);
  const costSold = proportionalCost(costBasis, quantity, position);
  const realizedGain = proceeds - costSold;
  return {
    gross,
    proceeds,
    costSold,
    realizedGain,
    realizedPct: costSold > 0 ? (realizedGain / costSold) * 100 : 0,
    closesPosition: Math.abs(quantity - position) <= QUANTITY_EPSILON,
  };
}

/** Total cost of a purchase: quantity x price plus the fee. */
export function buyTotal(
  quantity: number,
  pricePerUnit: number,
  fee: number
): number {
  const gross = lineAmount(quantity, pricePerUnit);
  return gross > 0 ? gross + Math.max(fee, 0) : 0;
}

/** Net dividend: gross minus withholding and fees, never below zero. */
export function netDividend(amount: number, fee: number): number {
  return Math.max(amount - Math.max(fee, 0), 0);
}

export type TradeKind = "buy" | "sell" | "dividend" | "split";

export interface TradeFormValues {
  kind: TradeKind;
  date: string;
  quantity: number;
  pricePerUnit: number;
  fee: number;
  /** Dividend gross amount, cents. */
  amount: number;
  splitRatio: number;
  /** Units held now (sell limit). */
  position: number;
}

/** First problem with a trade form, as a sentence, or `null` when it can be sent. */
export function validateTrade(v: TradeFormValues): string | null {
  if (!v.date) return "Date is required";
  switch (v.kind) {
    case "buy":
    case "sell":
      if (!(v.quantity > 0)) return "Quantity must be greater than zero";
      if (v.kind === "sell" && v.quantity > v.position + QUANTITY_EPSILON) {
        return `Cannot sell more than you hold (${v.position.toLocaleString(undefined, { maximumFractionDigits: 6 })} units)`;
      }
      if (!(v.pricePerUnit > 0)) return "Price per unit must be greater than zero";
      return null;
    case "dividend":
      if (!(v.amount > 0)) return "Amount must be greater than zero";
      if (v.fee > v.amount) {
        return "Withholding / fees cannot exceed the dividend amount";
      }
      return null;
    case "split":
      if (!(v.splitRatio > 0)) return "Split ratio must be greater than zero";
      return null;
  }
}
