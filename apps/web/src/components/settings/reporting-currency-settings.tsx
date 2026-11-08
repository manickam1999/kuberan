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
import { ApiClientError } from "@/lib/api-client";

/** Currencies offered as a reporting currency (the API validates support). */
export const REPORTING_CURRENCIES: { code: string; name: string }[] = [
  { code: "MYR", name: "Malaysian Ringgit" },
  { code: "USD", name: "US Dollar" },
  { code: "SGD", name: "Singapore Dollar" },
  { code: "GBP", name: "British Pound" },
  { code: "EUR", name: "Euro" },
  { code: "AUD", name: "Australian Dollar" },
  { code: "HKD", name: "Hong Kong Dollar" },
  { code: "CNY", name: "Chinese Yuan" },
  { code: "CAD", name: "Canadian Dollar" },
  { code: "NZD", name: "New Zealand Dollar" },
  { code: "CHF", name: "Swiss Franc" },
  { code: "THB", name: "Thai Baht" },
  { code: "IDR", name: "Indonesian Rupiah" },
  { code: "INR", name: "Indian Rupee" },
];

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
        {errorMessage && (
          <p role="alert" className="text-sm text-destructive">
            {errorMessage}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
