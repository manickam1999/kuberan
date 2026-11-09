import type { Investment } from "@/types/models";

/**
 * Per-holding figures. Native figures (security currency) and base figures
 * (user's reporting currency) are never combined: each P&L subtracts a cost
 * basis from a value in the same currency.
 */
export interface HoldingFigures {
  /** Security currency. */
  currency: string;
  /** quantity x current_price_native, in `currency`. */
  nativeValue: number;
  /** quantity x current_price, in the base currency; null without an FX rate. */
  baseValue: number | null;
  /** nativeValue - cost_basis, in `currency`. */
  nativePnl: number;
  /** baseValue - cost_basis_base, in the base currency; null without an FX rate. */
  basePnl: number | null;
}

/** Computes the native and base value / unrealized P&L of an open holding. */
export function holdingFigures(inv: Investment): HoldingFigures {
  const nativeValue = Math.round(inv.quantity * inv.current_price_native);
  const baseValue = inv.fx_missing
    ? null
    : Math.round(inv.quantity * inv.current_price);
  return {
    currency: inv.price_currency || inv.currency,
    nativeValue,
    baseValue,
    nativePnl: nativeValue - inv.cost_basis,
    basePnl: baseValue === null ? null : baseValue - inv.cost_basis_base,
  };
}

/**
 * Unrealised return of a holding as a percentage of its cost, in the base
 * currency when there is a rate and in the security's currency otherwise.
 * 0 when there is no cost.
 */
export function holdingReturnPct(inv: Investment): number {
  const f = holdingFigures(inv);
  const pnl = f.basePnl ?? f.nativePnl;
  const cost = f.basePnl === null ? inv.cost_basis : inv.cost_basis_base;
  return cost > 0 ? (pnl / cost) * 100 : 0;
}
