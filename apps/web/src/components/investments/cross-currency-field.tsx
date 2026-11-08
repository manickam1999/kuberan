"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CurrencyInput } from "@/components/ui/currency-input";
import { useBaseCurrency } from "@/hooks/use-profile";
import { convertCents, useFxLatest } from "@/hooks/use-fx-rates";
import type { FxAmountRequired } from "@/lib/api-client";
import type { Account } from "@/types/models";

interface CrossCurrencyAmountOptions {
  /** Currency of the trade (the security's). */
  securityCurrency: string;
  /** The funding / deposit cash account, if one is selected. */
  account: Account | undefined;
  /** What the trade moves, in cents of `securityCurrency` (used for the suggestion). */
  nativeTotal: number;
}

/**
 * State of the "amount in the account's currency" field shown when a trade is
 * settled through a cash account in another currency. The value is the user's
 * edit if there is one, else the server's `suggested_amount` after an
 * `FX_AMOUNT_REQUIRED`, else `nativeTotal` converted at `/fx-rates/latest`.
 */
export function useCrossCurrencyAmount({
  securityCurrency,
  account,
  nativeTotal,
}: CrossCurrencyAmountOptions) {
  const [manual, setManual] = useState<number | null>(null);
  const [hint, setHint] = useState<FxAmountRequired | null>(null);

  const accountCurrency = account?.currency;
  const differs =
    !!account && !!accountCurrency && accountCurrency !== securityCurrency;
  // The server can also demand the field (FX_AMOUNT_REQUIRED) when an account is chosen.
  const needed = !!account && (differs || hint !== null);
  const currency = differs ? accountCurrency : hint?.currency || accountCurrency;

  const rate = useFxLatest(
    differs ? securityCurrency : undefined,
    differs ? accountCurrency : undefined
  );
  const fromRate =
    nativeTotal > 0 ? convertCents(nativeTotal, rate.data?.rate) : null;
  const suggestion = hint?.suggestedAmount ?? fromRate;

  return {
    needed,
    currency: currency ?? "",
    value: manual ?? suggestion ?? 0,
    suggestion,
    isManual: manual !== null,
    rateLoading: differs && rate.isLoading,
    rateUnavailable: differs && rate.isError,
    setValue: (cents: number) => setManual(cents),
    acceptSuggestion: () => setManual(null),
    applyHint: (h: FxAmountRequired) => setHint(h),
    reset: () => {
      setManual(null);
      setHint(null);
    },
  };
}

interface CrossCurrencyAmountFieldProps {
  id: string;
  label: string;
  state: ReturnType<typeof useCrossCurrencyAmount>;
  disabled?: boolean;
}

/** Required, editable amount in the account's currency, pre-filled with a suggestion. */
export function CrossCurrencyAmountField({
  id,
  label,
  state,
  disabled,
}: CrossCurrencyAmountFieldProps) {
  if (!state.needed) return null;
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <CurrencyInput
        id={id}
        value={state.value}
        onChange={state.setValue}
        symbol={state.currency}
        placeholder="0.00"
        disabled={disabled}
        required
      />
      <p className="text-muted-foreground text-xs">
        {state.rateLoading
          ? "Looking up the exchange rate..."
          : state.rateUnavailable && state.suggestion === null
            ? `No exchange rate available; enter the ${state.currency} amount.`
            : "Suggested from the latest exchange rate; edit it to the amount your account actually moved."}
        {state.isManual && state.suggestion !== null && (
          <>
            {" "}
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto p-0 text-xs"
              onClick={state.acceptSuggestion}
            >
              Use suggestion
            </Button>
          </>
        )}
      </p>
    </div>
  );
}

const RATE_PATTERN = /^\d+(\.\d{1,10})?$/;

/** True when `value` is empty or a positive decimal with at most 10 fractional digits. */
export function isValidFxRate(value: string): boolean {
  const v = value.trim();
  return v === "" || (RATE_PATTERN.test(v) && Number(v) > 0);
}

interface FxRateFieldProps {
  id: string;
  securityCurrency: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}

/**
 * Optional "Exchange rate" (advanced) override: `1 <security currency> = ? <base>`.
 * Hidden for base-currency securities. Maps to the request's `fx_rate`.
 */
export function FxRateField({
  id,
  securityCurrency,
  value,
  onChange,
  disabled,
}: FxRateFieldProps) {
  const baseCurrency = useBaseCurrency();
  const [open, setOpen] = useState(false);
  const foreign = !!securityCurrency && securityCurrency !== baseCurrency;
  const latest = useFxLatest(
    foreign ? securityCurrency : undefined,
    foreign ? baseCurrency : undefined
  );
  if (!foreign) return null;

  if (!open && value === "") {
    return (
      <Button
        type="button"
        variant="link"
        size="sm"
        className="h-auto self-start p-0 text-xs"
        onClick={() => setOpen(true)}
      >
        Set exchange rate (advanced)
      </Button>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>
        Exchange rate (optional): 1 {securityCurrency} = ? {baseCurrency}
      </Label>
      <Input
        id={id}
        inputMode="decimal"
        placeholder={latest.data?.rate ?? "Market rate"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      />
      <p className="text-muted-foreground text-xs">
        Leave empty to use the rate implied by your account, or the market rate.
      </p>
    </div>
  );
}
