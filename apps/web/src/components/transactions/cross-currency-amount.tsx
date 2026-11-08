"use client";

import { useState } from "react";
import { useFxLatest, convertCents } from "@/hooks/use-fx-rates";
import { Label } from "@/components/ui/label";
import { CurrencyInput } from "@/components/ui/currency-input";

/** `1 MYR = 0.2466 USD` for an amount pair, or null when it can't be computed. */
export function formatImpliedRate(
  fromCurrency: string,
  fromCents: number,
  toCurrency: string,
  toCents: number
): string | null {
  if (fromCents <= 0 || toCents <= 0) return null;
  const rate = toCents / fromCents;
  const text = rate.toFixed(rate >= 1 ? 4 : 6).replace(/0+$/, "").replace(/\.$/, "");
  return `1 ${fromCurrency} = ${text} ${toCurrency}`;
}

export interface CrossAmountState {
  /** The two currencies differ, so the second amount is required. */
  needed: boolean;
  /** Current second amount in cents (user edit, else the market suggestion). */
  value: number;
  /** Set by the user (stops auto-prefill until the pair or source changes). */
  setValue: (cents: number) => void;
  /** Market rate behind the prefill, when one exists. */
  rate: string | undefined;
}

/**
 * State for a "second amount" field. The value follows the market suggestion
 * (`fromAmount x latest rate`) until the user edits it, or a caller sets it
 * (e.g. from an FX_AMOUNT_REQUIRED suggestion). An edit is tied to the
 * currency pair, so changing an account falls back to a fresh suggestion.
 */
export function useCrossAmount(
  fromCurrency: string,
  toCurrency: string,
  fromAmount: number
): CrossAmountState {
  const needed = !!fromCurrency && !!toCurrency && fromCurrency !== toCurrency;
  // Disabled for an empty or same-currency pair, like `needed`.
  const { data } = useFxLatest(fromCurrency, toCurrency);
  const [manual, setManual] = useState<{ pair: string; value: number } | null>(
    null
  );
  const pair = `${fromCurrency}>${toCurrency}`;
  const suggested =
    needed && data?.rate && fromAmount > 0
      ? (convertCents(fromAmount, data.rate) ?? 0)
      : 0;
  const value = manual && manual.pair === pair ? manual.value : suggested;
  return {
    needed,
    value,
    setValue: (cents) => setManual({ pair, value: cents }),
    rate: data?.rate,
  };
}

interface CrossCurrencyAmountProps {
  id: string;
  label: string;
  fromCurrency: string;
  fromAmount: number;
  toCurrency: string;
  value: number;
  onChange: (cents: number) => void;
  rate?: string;
  disabled?: boolean;
}

/**
 * Required input for the amount that lands in a different currency, with the
 * implied rate shown live beneath it.
 */
export function CrossCurrencyAmount({
  id,
  label,
  fromCurrency,
  fromAmount,
  toCurrency,
  value,
  onChange,
  rate,
  disabled,
}: CrossCurrencyAmountProps) {
  const implied = formatImpliedRate(fromCurrency, fromAmount, toCurrency, value);
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <CurrencyInput
        id={id}
        value={value}
        onChange={onChange}
        currency={toCurrency}
        placeholder="0.00"
        disabled={disabled}
        required
        className="money"
      />
      <p className="text-xs text-muted-foreground">
        {implied ? `Implied rate: ${implied}` : "Enter the amount received."}
        {rate && ` · Market rate: 1 ${fromCurrency} = ${rate} ${toCurrency}`}
      </p>
    </div>
  );
}
