package services

import (
	"math"

	apperrors "kuberan/internal/errors"
)

// maxLineAmountCents is the largest quantity × unit price (in cents) accepted
// on a write: 1e15 cents (10 trillion in major units). It keeps the result far
// inside int64 and float64's exact-integer range (2^53 ≈ 9e15).
const maxLineAmountCents = 1e15

// valueCents returns quantity × unitPrice rounded to the nearest cent. It is
// used on read paths (market values) where inputs were validated on write.
func valueCents(quantity float64, unitPrice int64) int64 {
	return int64(math.Round(quantity * float64(unitPrice)))
}

// lineAmount returns quantity × unitPrice rounded to the nearest cent,
// rejecting non-finite or out-of-range products with a validation error.
func lineAmount(quantity float64, unitPrice int64) (int64, error) {
	product := quantity * float64(unitPrice)
	if math.IsNaN(product) || math.IsInf(product, 0) || math.Abs(product) > maxLineAmountCents {
		return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "quantity × price is too large")
	}
	return int64(math.Round(product)), nil
}
