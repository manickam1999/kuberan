package services

import apperrors "kuberan/internal/errors"

// isTwoDecimalCurrency reports whether code (already normalised) is a currency
// whose minor unit is 1/100. Amounts are stored as cents, so currencies with
// another minor unit (JPY, KWD, ...) are not supported (plan decision Q8).
func isTwoDecimalCurrency(code string) bool {
	_, bad := fxNonTwoDecimalCurrencies[code]
	return !bad
}

// validateCreatableCurrency rejects a currency that cannot hold cent amounts.
// An empty code is left to the caller's defaulting.
func validateCreatableCurrency(code string) error {
	if code == "" {
		return nil
	}
	if !isTwoDecimalCurrency(normalizeCode(code)) {
		return apperrors.ErrUnsupportedCurrency
	}
	return nil
}
