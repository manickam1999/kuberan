package mcp

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"kuberan/internal/services"
)

// unknownCurrency is ISO 4217's code for "no currency", printed when a record
// carries none, so an amount never appears without a unit.
const unknownCurrency = "XXX"

// formatMoney renders int64 minor units as "<ISO code> <amount>", for example
// "MYR 1,234.56" or "USD -12.00". The amount is grouped by thousands. ISO codes
// are used instead of symbols because "$" is ambiguous and models read codes
// reliably. All supported currencies have two decimals.
func formatMoney(cents int64, currency string) string {
	code := currencyCode(currency)
	if code == "" {
		code = unknownCurrency
	}
	negative := cents < 0
	// uint64 negation is safe for math.MinInt64.
	abs := uint64(cents)
	if negative {
		abs = -abs
	}
	whole := strconv.FormatUint(abs/100, 10)
	var grouped strings.Builder
	for i, ch := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(ch)
	}
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s %s%s.%02d", code, sign, grouped.String(), abs%100)
}

// currencyCode normalizes a currency code for printing: upper case, letters and
// digits only, at most 8 characters. Codes are validated server-side; this keeps
// a malformed stored value from putting free text into tool output.
func currencyCode(currency string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(currency) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			if sb.Len() == 8 {
				break
			}
		}
	}
	return sb.String()
}

// moneyWithBase renders a native amount followed by its base-currency value,
// "USD 1,866.40 (≈ MYR 7,571.10)". The base part is omitted when the amount is
// already in the base currency, and reads "(no exchange rate)" when it should
// have one but base is nil.
func moneyWithBase(cents int64, currency string, base *int64, baseCurrency string) string {
	native := formatMoney(cents, currency)
	if strings.EqualFold(currency, baseCurrency) {
		return native
	}
	if base == nil {
		return native + " (no exchange rate)"
	}
	return native + " (≈ " + formatMoney(*base, baseCurrency) + ")"
}

// fxNotes renders the notes for a converted response: one line naming each
// native amount left out of the totals for lack of an exchange rate, or a
// generic line when the response only says fx_missing.
func fxNotes(fxMissing bool, unconverted []services.UnconvertedAmount) string {
	var sb strings.Builder
	for _, u := range unconverted {
		fmt.Fprintf(&sb, "Note: excludes %s (no exchange rate)\n", formatMoney(u.Amount, u.Currency))
	}
	if fxMissing && len(unconverted) == 0 {
		sb.WriteString("Note: some amounts are excluded (no exchange rate)\n")
	}
	return sb.String()
}

// dollarsToCents converts a float64 dollar amount to int64 cents.
func dollarsToCents(dollars float64) int64 {
	return int64(math.Round(dollars * 100))
}
