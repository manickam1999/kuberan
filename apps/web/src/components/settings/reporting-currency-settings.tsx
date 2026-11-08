"use client";

import { toast } from "sonner";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useBaseCurrency, useSetBaseCurrency } from "@/hooks/use-profile";
import { REPORTING_CURRENCIES } from "@/lib/currencies";
import { ApiClientError } from "@/lib/api-client";

export function ReportingCurrencySettings() {
  const baseCurrency = useBaseCurrency();
  const setBaseCurrency = useSetBaseCurrency();

  const options = REPORTING_CURRENCIES.some((c) => c.code === baseCurrency)
    ? REPORTING_CURRENCIES
    : [{ code: baseCurrency, name: baseCurrency }, ...REPORTING_CURRENCIES];

  const error = setBaseCurrency.error;
  const errorMessage = error
    ? error instanceof ApiClientError && error.code === "UNSUPPORTED_CURRENCY"
      ? "That currency isn't supported: there is no exchange rate to convert it."
      : error instanceof ApiClientError && error.code === "BASE_CURRENCY_LOCKED"
        ? error.message
        : error instanceof ApiClientError
        ? error.message
        : "Failed to update reporting currency"
    : null;

  function handleChange(value: string) {
    if (value === baseCurrency) return;
    setBaseCurrency.mutate(value, {
      onSuccess: () => toast.success(`Reporting currency set to ${value}`),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Reporting currency</CardTitle>
        <CardDescription>
          Totals, net worth, budgets and analytics are converted to this
          currency. Each account and holding keeps its own currency.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Select
          value={baseCurrency}
          onValueChange={handleChange}
          disabled={setBaseCurrency.isPending}
        >
          <SelectTrigger className="w-full sm:w-72" aria-label="Reporting currency">
            <SelectValue placeholder="Select currency" />
          </SelectTrigger>
          <SelectContent>
            {options.map((c) => (
              <SelectItem key={c.code} value={c.code}>
                {c.code} · {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          The reporting currency is locked once you have investment history.
        </p>
        {errorMessage && (
          <p role="alert" className="text-sm text-destructive">
            {errorMessage}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
