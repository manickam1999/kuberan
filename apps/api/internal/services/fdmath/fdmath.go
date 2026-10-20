// Package fdmath provides pure interest-math helpers for fixed deposit accounts.
//
// All monetary inputs and outputs are int64 cents (e.g. $10.50 = 1050).
// Rates are annual percentages as float64 (e.g. 3.5 means 3.5 %/yr, i.e. r = 0.035).
//
// # Day-count convention
//
// Actual/365-fixed: the number of calendar days between two dates is counted exactly
// (including leap-day occurrences), and the year denominator is always 365. This is
// simple, deterministic, and consistent with how most retail fixed-deposit products
// quote rates in common jurisdictions.
//
// Day differences are computed on UTC calendar dates (hour/minute/second stripped)
// so that neither DST transitions nor the time-of-day at which a record is created
// can affect the day count.
//
// # Rounding convention
//
// Half-up (round half away from zero toward positive infinity). A result of X.5 cents
// rounds to X+1. This is the convention most users expect for financial display values.
// A single internal helper roundCents is used for every money computation; callers
// do not need to round independently.
package fdmath

import (
	"math"
	"time"
)

// roundCents rounds a float64 cent value to the nearest cent using half-up rounding.
// Negative values are not expected in normal usage but are handled correctly.
func roundCents(v float64) int64 {
	return int64(math.Floor(v + 0.5))
}

// daysBetween returns the number of whole calendar days from start to end using
// UTC date arithmetic (time-of-day components are ignored).
// Returns 0 if end <= start.
func daysBetween(start, end time.Time) int64 {
	// Truncate to UTC date boundaries so clock time and DST cannot skew the count.
	s := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	e := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	d := e.Sub(s)
	if d <= 0 {
		return 0
	}
	return int64(d.Hours() / 24)
}

// MaturityValue computes the cumulative compound maturity value of a fixed deposit.
//
// Formula: MV = round(P * (1 + r/n)^(n*t))
//
// where:
//   - P = principal in cents
//   - r = annualRatePercent / 100
//   - n = compoundingPeriodsPerYear (must be > 0; if <= 0 the principal is returned unchanged)
//   - t = actual days from start to maturity / 365 (actual/365-fixed day count)
//
// A zero or negative annualRatePercent returns the principal unchanged (no growth).
// A zero or negative principal returns 0.
func MaturityValue(principal int64, annualRatePercent float64, start, maturity time.Time, compoundingPeriodsPerYear int) int64 {
	if principal <= 0 {
		return 0
	}
	if annualRatePercent <= 0 || compoundingPeriodsPerYear <= 0 {
		return principal
	}
	days := daysBetween(start, maturity)
	if days <= 0 {
		return principal
	}
	r := annualRatePercent / 100.0
	n := float64(compoundingPeriodsPerYear)
	t := float64(days) / 365.0
	mv := float64(principal) * math.Pow(1.0+r/n, n*t)
	return roundCents(mv)
}

// PeriodicInterest computes the interest payment for a single payout period.
//
// Formula: interest = round(P * r / periodsPerYear)
//
// where r = annualRatePercent / 100. This is the simple (non-compounded) amount paid
// each period when the payout type is "periodic".
//
// Returns 0 if annualRatePercent <= 0, periodsPerYear <= 0, or principal <= 0.
func PeriodicInterest(principal int64, annualRatePercent float64, periodsPerYear int) int64 {
	if principal <= 0 || annualRatePercent <= 0 || periodsPerYear <= 0 {
		return 0
	}
	r := annualRatePercent / 100.0
	n := float64(periodsPerYear)
	return roundCents(float64(principal) * r / n)
}

// AccruedInterest computes the simple pro-rata interest accrued from start to asOf.
//
// Formula: accrued = round(P * r * daysElapsed / 365)
//
// where r = annualRatePercent / 100 and daysElapsed = calendar days from start to asOf
// (actual/365-fixed). Returns 0 if asOf <= start or annualRatePercent <= 0 or principal <= 0.
//
// This is used to suggest a realized amount for early-close dialogs; the caller is
// responsible for clamping to the deposit term if desired.
func AccruedInterest(principal int64, annualRatePercent float64, start, asOf time.Time) int64 {
	if principal <= 0 || annualRatePercent <= 0 {
		return 0
	}
	days := daysBetween(start, asOf)
	if days <= 0 {
		return 0
	}
	r := annualRatePercent / 100.0
	return roundCents(float64(principal) * r * float64(days) / 365.0)
}

// PayoutFrequencyPeriods maps a payout-frequency string to the number of payout
// periods per year. The recognized values are:
//
//	"monthly"    -> 12
//	"quarterly"  -> 4
//	"semiannual" -> 2
//	"annual"     -> 1
//
// The second return value is false for any unrecognized string.
// The mapping is intentionally kept on plain strings so this package has no
// dependency on the models enums package.
func PayoutFrequencyPeriods(frequency string) (int, bool) {
	switch frequency {
	case "monthly":
		return 12, true
	case "quarterly":
		return 4, true
	case "semiannual":
		return 2, true
	case "annual":
		return 1, true
	default:
		return 0, false
	}
}
