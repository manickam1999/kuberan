package services

import (
	"errors"
	"time"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
)

// fxAmountRequiredError builds the FX_AMOUNT_REQUIRED error for an operation
// that moves amount (in currency from) into an account in currency to and needs
// the caller to state the amount in currency to. field names the request field
// to fill. When a market rate exists the details carry a suggestion
// (suggested_amount, rate, rate_recorded_at); without one only currency and
// field. fxs may be nil. Errors other than "no rate" are returned as is.
func fxAmountRequiredError(fxs FXServicer, from, to string, amount int64, at time.Time, field string) error {
	details := map[string]any{"currency": to, "field": field}
	if fxs != nil && amount > 0 {
		q, err := fxs.Rate(from, to, at)
		switch {
		case err == nil:
			if suggested, cerr := fx.Convert(amount, q.Rate); cerr == nil && suggested > 0 {
				details["suggested_amount"] = suggested
				details["rate"] = q.Rate.String()
				details["rate_recorded_at"] = q.RecordedAt.UTC().Format(time.RFC3339)
			}
		case !errors.Is(err, apperrors.ErrFXRateUnavailable):
			return err
		}
	}
	return apperrors.WithDetails(apperrors.ErrFXAmountRequired, details)
}
