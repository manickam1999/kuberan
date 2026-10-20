package fdmath

import (
	"testing"
	"time"
)

// date is a convenience helper that returns a UTC midnight time for the given date.
func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// daysBetween helper tests
// ---------------------------------------------------------------------------

func TestDaysBetween(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  int64
	}{
		{
			name:  "same day returns 0",
			start: date(2024, 1, 1),
			end:   date(2024, 1, 1),
			want:  0,
		},
		{
			name:  "end before start returns 0",
			start: date(2024, 6, 1),
			end:   date(2024, 1, 1),
			want:  0,
		},
		{
			name:  "one day",
			start: date(2024, 1, 1),
			end:   date(2024, 1, 2),
			want:  1,
		},
		{
			name:  "one year non-leap (2023-01-01 to 2024-01-01 = 365 days)",
			start: date(2023, 1, 1),
			end:   date(2024, 1, 1),
			want:  365,
		},
		{
			name:  "leap year span (2024-01-01 to 2025-01-01 = 366 days)",
			start: date(2024, 1, 1),
			end:   date(2025, 1, 1),
			want:  366,
		},
		{
			name:  "ignores clock time - start has noon, end has midnight",
			start: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			end:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			want:  30,
		},
		{
			name:  "ignores clock time - start midnight, end has 23:59",
			start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
			want:  30,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := daysBetween(tc.start, tc.end)
			if got != tc.want {
				t.Errorf("daysBetween(%v, %v) = %d; want %d", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// roundCents helper tests
// ---------------------------------------------------------------------------

func TestRoundCents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   float64
		want int64
	}{
		{0.0, 0},
		{1.0, 1},
		{1.4, 1}, // below 0.5 -> round down
		{1.5, 2}, // exactly 0.5 -> round up (half-up)
		{1.6, 2}, // above 0.5 -> round up
		{2.5, 3}, // half-up on 2.5
		{99.499, 99},
		{99.5, 100}, // half-up boundary
		{100.0, 100},
	}

	for _, tc := range tests {
		got := roundCents(tc.in)
		if got != tc.want {
			t.Errorf("roundCents(%v) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// MaturityValue tests
// ---------------------------------------------------------------------------

func TestMaturityValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal int64
		rate      float64
		start     time.Time
		maturity  time.Time
		compPerYr int
		want      int64
		// allowDelta is for cases where we want to allow ±1 cent for floating-point
		// imprecision in hand calculations. 0 means exact match required.
		allowDelta int64
	}{
		// --- Zero / edge cases ---
		{
			name:      "zero rate returns principal",
			principal: 100000, // $1000.00
			rate:      0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: 12,
			want:      100000,
		},
		{
			name:      "negative rate returns principal",
			principal: 100000,
			rate:      -1.0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: 12,
			want:      100000,
		},
		{
			name:      "zero principal returns 0",
			principal: 0,
			rate:      5.0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: 12,
			want:      0,
		},
		{
			name:      "negative compoundingPeriodsPerYear returns principal",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: -1,
			want:      100000,
		},
		{
			name:      "zero compoundingPeriodsPerYear returns principal",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: 0,
			want:      100000,
		},
		{
			name:      "maturity equals start returns principal",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 12,
			want:      100000,
		},
		{
			name:      "maturity before start returns principal",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 6, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 12,
			want:      100000,
		},

		// --- Annual compounding (n=1), exactly 1 year ---
		// P=100000 ($1000.00), r=0.05, n=1, t=1 (non-leap 2023)
		// MV = 100000 * (1.05)^1 = 105000 exactly
		{
			name:      "annual compounding 5% 1yr non-leap exact",
			principal: 100000,
			rate:      5.0,
			start:     date(2023, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 1,
			want:      105000,
		},

		// --- Annual compounding (n=1), leap year span (366 days) ---
		// P=100000, r=0.05, n=1, t=366/365
		// MV = 100000 * (1.05)^(366/365)
		// (1.05)^(366/365) = exp((366/365)*ln(1.05))
		// ln(1.05) ≈ 0.04879016417
		// (366/365)*0.04879016417 ≈ 1.002739726*0.04879016417 ≈ 0.04892373
		// exp(0.04892373) ≈ 1.050137...
		// MV ≈ 105014 cents (rounded) -- let's verify by computer precision
		// Using more precision: math.Pow(1.05, 366.0/365.0)*100000
		// = 1.0501368...*100000 = 105013.68... -> rounds to 105014
		{
			name:      "annual compounding 5% leap-year span (366 days)",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			maturity:  date(2025, 1, 1),
			compPerYr: 1,
			want:      105014, // 100000 * 1.05^(366/365) ≈ 105013.68 -> 105014
		},

		// --- Semiannual compounding (n=2), exactly 1 year ---
		// P=100000, r=0.05, n=2, t=1 (365 days / 365)
		// MV = 100000 * (1 + 0.05/2)^(2*1) = 100000 * (1.025)^2
		// In float64: 1.025^2 = 1.05062499999999992... (slightly below mathematical 1.050625)
		// 100000 * 1.05062499... = 105062.499... -> rounds down to 105062
		{
			name:      "semiannual compounding 5% 1yr",
			principal: 100000,
			rate:      5.0,
			start:     date(2023, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 2,
			want:      105062, // float64: 100000 * 1.025^2 = 105062.4999... -> 105062
		},

		// --- Quarterly compounding (n=4), exactly 1 year ---
		// P=100000, r=0.05, n=4, t=1
		// MV = 100000 * (1.0125)^4 = 100000 * 1.05094534...
		// (1.0125)^4 = 1.01250000^4
		// 1.0125^2 = 1.02515625
		// 1.02515625^2 = 1.05094534
		// MV = 105094.534... -> 105095
		{
			name:      "quarterly compounding 5% 1yr",
			principal: 100000,
			rate:      5.0,
			start:     date(2023, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 4,
			want:      105095, // 100000 * (1.0125)^4 ≈ 105094.534 -> 105095
		},

		// --- Monthly compounding (n=12), exactly 1 year ---
		// P=100000, r=0.05, n=12, t=1
		// MV = 100000 * (1 + 0.05/12)^12
		// (1 + 0.004166...)^12
		// well-known: ≈ 1.051161898...
		// MV = 105116.189... -> 105116
		{
			name:      "monthly compounding 5% 1yr",
			principal: 100000,
			rate:      5.0,
			start:     date(2023, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 12,
			want:      105116, // 100000 * (1+0.05/12)^12 ≈ 105116.19 -> 105116
		},

		// --- Sub-year term (6 months, ~182 days) ---
		// P=200000 ($2000.00), r=0.04, n=4, t=182/365
		// (1 + 0.04/4)^(4 * 182/365) = (1.01)^(1.99452...)
		// (1.01)^(1.99452) = exp(1.99452 * ln(1.01)) = exp(1.99452 * 0.00995033) ≈ exp(0.019852) ≈ 1.020050
		// MV = 200000 * 1.020050 ≈ 204010
		{
			name:       "quarterly 4% 6-month sub-year",
			principal:  200000,
			rate:       4.0,
			start:      date(2024, 1, 1),
			maturity:   date(2024, 7, 1), // 182 days
			compPerYr:  4,
			want:       204010, // verified: 200000*(1.01)^(4*182/365)
			allowDelta: 1,
		},

		// --- Multi-year term (3 years, annual compounding) ---
		// P=500000 ($5000.00), r=0.06, n=1, t=3 (using 3*365=1095 days span)
		// MV = 500000 * (1.06)^3 = 500000 * 1.191016 = 595508
		{
			name:      "annual 6% 3yr multi-year",
			principal: 500000,
			rate:      6.0,
			start:     date(2021, 1, 1),
			maturity:  date(2024, 1, 1), // 1096 days (2024 is not leap but 2024-01-01 - 2021-01-01 spans a leap year 2024... wait: 2021+365=2022, 2022+365=2023, 2023+365=2024; no leap year 2021/2022/2023 are non-leap; 2024 is leap but we don't count it; so 3*365=1095)
			compPerYr: 1,
			// t = 1095/365 = 3.0 exactly
			// MV = 500000 * (1.06)^3 = 500000 * 1.191016 = 595508.0 -> 595508
			want: 595508,
		},

		// --- 3.5% rate, monthly compounding, 2 years ---
		// P=1000000 ($10000.00), r=0.035, n=12, t=730/365=2
		// 2022-01-01 to 2024-01-01 = 730 days (2022 and 2023 are non-leap); t=2.0 exactly
		// float64: 1000000 * (1+0.035/12)^24 = 1072398.914... -> rounds to 1072399
		{
			name:      "monthly 3.5% 2yr",
			principal: 1000000,
			rate:      3.5,
			start:     date(2022, 1, 1),
			maturity:  date(2024, 1, 1), // 730 days; t=2.0 exactly
			compPerYr: 12,
			want:      1072399, // float64: 1072398.914... -> 1072399
		},

		// --- Rounding: choose principal and rate so result lands on exact .5 cent boundary ---
		// We want P * (1 + r/n)^(n*t) = X.5 for some integer X.
		// Simple approach: n=1, t=1 (365 days non-leap), r such that P*(1+r) = X.5
		// P=1, rate=50% => MV = 1 * 1.5 = 1.5 -> rounds to 2
		{
			name:      "rounding half-up: 1 cent at 50% annual for 1 yr = 1.5 -> 2",
			principal: 1,
			rate:      50.0,
			start:     date(2023, 1, 1),
			maturity:  date(2024, 1, 1),
			compPerYr: 1,
			want:      2, // 1 * 1.5 = 1.5, half-up -> 2
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MaturityValue(tc.principal, tc.rate, tc.start, tc.maturity, tc.compPerYr)
			diff := got - tc.want
			if diff < 0 {
				diff = -diff
			}
			if diff > tc.allowDelta {
				t.Errorf("MaturityValue(%d, %v, %v, %v, %d) = %d; want %d (allowDelta=%d)",
					tc.principal, tc.rate, tc.start.Format("2006-01-02"), tc.maturity.Format("2006-01-02"),
					tc.compPerYr, got, tc.want, tc.allowDelta)
			}
		})
	}
}

// TestMaturityValueLeapYearDayCount asserts explicitly that the leap year 2024
// yields a 366-day span and that this affects the result differently from a 365-day span.
func TestMaturityValueLeapYearDayCount(t *testing.T) {
	t.Parallel()

	// 2024-01-01 to 2025-01-01 = 366 days (2024 is a leap year)
	leapDays := daysBetween(date(2024, 1, 1), date(2025, 1, 1))
	if leapDays != 366 {
		t.Fatalf("expected 366 days for 2024 leap-year span, got %d", leapDays)
	}

	// 2023-01-01 to 2024-01-01 = 365 days
	nonLeapDays := daysBetween(date(2023, 1, 1), date(2024, 1, 1))
	if nonLeapDays != 365 {
		t.Fatalf("expected 365 days for non-leap span, got %d", nonLeapDays)
	}

	// Maturity value should be strictly greater for 366 days vs 365 days (same rate, same principal).
	mvLeap := MaturityValue(100000, 5.0, date(2024, 1, 1), date(2025, 1, 1), 1)
	mvNonLeap := MaturityValue(100000, 5.0, date(2023, 1, 1), date(2024, 1, 1), 1)
	if mvLeap <= mvNonLeap {
		t.Errorf("leap-year MV (%d) should be > non-leap MV (%d)", mvLeap, mvNonLeap)
	}
}

// ---------------------------------------------------------------------------
// PeriodicInterest tests
// ---------------------------------------------------------------------------

func TestPeriodicInterest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal int64
		rate      float64
		perYear   int
		want      int64
	}{
		// Zero / edge cases
		{
			name:      "zero rate returns 0",
			principal: 100000,
			rate:      0,
			perYear:   12,
			want:      0,
		},
		{
			name:      "negative rate returns 0",
			principal: 100000,
			rate:      -2.0,
			perYear:   12,
			want:      0,
		},
		{
			name:      "zero principal returns 0",
			principal: 0,
			rate:      5.0,
			perYear:   12,
			want:      0,
		},
		{
			name:      "zero periodsPerYear returns 0",
			principal: 100000,
			rate:      5.0,
			perYear:   0,
			want:      0,
		},
		{
			name:      "negative periodsPerYear returns 0",
			principal: 100000,
			rate:      5.0,
			perYear:   -1,
			want:      0,
		},

		// Monthly (12 per year)
		// P=120000 ($1200), r=0.05 => annual interest=6000, monthly=500 exact
		{
			name:      "monthly 5% P=120000 exact",
			principal: 120000,
			rate:      5.0,
			perYear:   12,
			want:      500, // 120000 * 0.05 / 12 = 500.0 exactly
		},

		// Quarterly (4 per year)
		// P=100000, r=0.04 => annual=4000, quarterly=1000 exact
		{
			name:      "quarterly 4% P=100000 exact",
			principal: 100000,
			rate:      4.0,
			perYear:   4,
			want:      1000, // 100000 * 0.04 / 4 = 1000.0
		},

		// Semiannual (2 per year)
		// P=200000, r=0.06 => annual=12000, semiannual=6000 exact
		{
			name:      "semiannual 6% P=200000 exact",
			principal: 200000,
			rate:      6.0,
			perYear:   2,
			want:      6000, // 200000 * 0.06 / 2 = 6000.0
		},

		// Annual (1 per year)
		// P=100000, r=0.035 => annual=3500 exact
		{
			name:      "annual 3.5% P=100000",
			principal: 100000,
			rate:      3.5,
			perYear:   1,
			want:      3500, // 100000 * 0.035 = 3500
		},

		// Monthly with rounding
		// P=100000, r=0.05 => annual=5000, monthly=416.666... -> 417
		{
			name:      "monthly 5% P=100000 rounds up",
			principal: 100000,
			rate:      5.0,
			perYear:   12,
			want:      417, // 100000 * 0.05 / 12 = 416.666... -> round half-up = 417
		},

		// Rounding exactly at .5: P=3, r=100%, perYear=6 => 3*1/6 = 0.5 -> 1
		{
			name:      "rounding half-up boundary: 0.5 cents -> 1",
			principal: 3,
			rate:      100.0,
			perYear:   6,
			want:      1, // 3 * 1.0 / 6 = 0.5 -> half-up = 1
		},

		// Rounding below .5: P=3, r=100%, perYear=7 => 3/7 = 0.4285... -> 0
		{
			name:      "rounding below half: 3/7 = 0.4285 -> 0",
			principal: 3,
			rate:      100.0,
			perYear:   7,
			want:      0, // 3 * 1.0 / 7 = 0.428... -> 0
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PeriodicInterest(tc.principal, tc.rate, tc.perYear)
			if got != tc.want {
				t.Errorf("PeriodicInterest(%d, %v, %d) = %d; want %d",
					tc.principal, tc.rate, tc.perYear, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AccruedInterest tests
// ---------------------------------------------------------------------------

func TestAccruedInterest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal int64
		rate      float64
		start     time.Time
		asOf      time.Time
		want      int64
	}{
		// asOf == start => 0
		{
			name:      "asOf equals start returns 0",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 1, 1),
			want:      0,
		},
		// asOf before start => 0
		{
			name:      "asOf before start returns 0",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 6, 1),
			asOf:      date(2024, 1, 1),
			want:      0,
		},
		// zero rate => 0
		{
			name:      "zero rate returns 0",
			principal: 100000,
			rate:      0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 7, 1),
			want:      0,
		},
		// negative rate => 0
		{
			name:      "negative rate returns 0",
			principal: 100000,
			rate:      -1.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 7, 1),
			want:      0,
		},
		// zero principal => 0
		{
			name:      "zero principal returns 0",
			principal: 0,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 7, 1),
			want:      0,
		},

		// At maturity (full term = 365 days), simple interest
		// P=100000, r=0.05, days=365 => 100000 * 0.05 * 365/365 = 5000 exact
		{
			name:      "full year simple interest exact",
			principal: 100000,
			rate:      5.0,
			start:     date(2023, 1, 1),
			asOf:      date(2024, 1, 1),
			want:      5000,
		},

		// Mid-term: 182 days (half year-ish)
		// P=100000, r=0.05, days=182 => 100000 * 0.05 * 182/365 = 5000 * 0.4986... = 2493.15... -> 2493
		{
			name:      "mid-term 182 days",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 7, 1), // 182 days
			want:      2493,             // 100000 * 0.05 * 182/365 = 2493.15... -> 2493
		},

		// One day accrual
		// P=365000, r=0.05, days=1 => 365000 * 0.05 / 365 = 50 exact
		{
			name:      "one day exact",
			principal: 365000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 1, 2),
			want:      50, // 365000 * 0.05 * 1/365 = 50.0 exactly
		},

		// Leap-year span: 2024-01-01 to 2025-01-01 = 366 days
		// P=100000, r=0.05, days=366 => 100000 * 0.05 * 366/365 = 5013.698... -> 5014
		{
			name:      "leap-year full span 366 days",
			principal: 100000,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2025, 1, 1),
			want:      5014, // 100000 * 0.05 * 366/365 = 5013.7 -> 5014
		},

		// Rounding half-up boundary
		// P=7300, r=5%, days=1 => 7300 * 0.05 * 1/365 = 365/365 = 1.0 exact
		{
			name:      "one day exactly 1 cent",
			principal: 7300,
			rate:      5.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 1, 2),
			want:      1, // 7300 * 0.05 / 365 = 1.0 exactly
		},
		// P=7, r=100%, days=1 => 7 * 1.0 * 1/365 = 0.01917... -> 0
		{
			name:      "sub-cent rounds to 0",
			principal: 7,
			rate:      100.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 1, 2),
			want:      0,
		},

		// Rounding: find a case that produces exactly .5
		// P * r * d / 365 = X.5
		// choose P=730, r=100%, d=1: 730 * 1 * 1/365 = 2.0 (not .5)
		// choose P=1460, r=100%, d=1: 1460/365 = 4 (not .5)
		// P=73, r=100%, d=1: 73/365 = 0.2 (not .5)
		// P=365*1+182 = 547, r=100%, d=1: 547/365 = 1.4986... (not .5)
		// Try: we need P*r*d/(365) = N.5; simplest: P=365*3, r=100%, d=1 => 3 (not .5)
		// P=365, r=100%, d=1 => 1.0 (exact)
		// We need remainder = 0.5: 365*k / 365 = k (integer). OK use d=2:
		// P=365, r=100%, d=2 => 2.0 (exact)
		// Try P=730+1=731, r=100%, d=1: 731/365 = 2.002... (not .5)
		// Let's compute: P*d mod 365 = 365/2 = 182.5 -- not integer, so exact .5 not achievable
		// with integer P, d=1. Use d=2, need P*2 mod 365 = 182 (since 365/2=182.5, won't work perfectly).
		// Instead: P*r*d/365 mod 1 = 0.5 means P*r*d = 365*(k+0.5) = 365k + 182.5
		// P*r*d must be 182.5. If r=100% and d=1, P=182.5 (non-integer). Not achievable with integers.
		// Use r=50%, d=1: P * 0.5 * 1/365 = 0.5 => P=365. Check: 365*0.5*1/365=0.5 -> rounds to 1.
		{
			name:      "rounding half-up: 0.5 cents -> 1",
			principal: 365,
			rate:      50.0,
			start:     date(2024, 1, 1),
			asOf:      date(2024, 1, 2),
			want:      1, // 365 * 0.5 * 1 / 365 = 0.5 -> half-up = 1
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := AccruedInterest(tc.principal, tc.rate, tc.start, tc.asOf)
			if got != tc.want {
				t.Errorf("AccruedInterest(%d, %v, %v, %v) = %d; want %d",
					tc.principal, tc.rate, tc.start.Format("2006-01-02"), tc.asOf.Format("2006-01-02"),
					got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PayoutFrequencyPeriods tests
// ---------------------------------------------------------------------------

func TestPayoutFrequencyPeriods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		freq   string
		wantN  int
		wantOK bool
	}{
		{"monthly", 12, true},
		{"quarterly", 4, true},
		{"semiannual", 2, true},
		{"annual", 1, true},
		// Unknown / invalid
		{"", 0, false},
		{"weekly", 0, false},
		{"daily", 0, false},
		{"MONTHLY", 0, false}, // case-sensitive
		{"semi-annual", 0, false},
		{"biannual", 0, false},
	}

	for _, tc := range tests {
		t.Run("freq_"+tc.freq, func(t *testing.T) {
			t.Parallel()
			gotN, gotOK := PayoutFrequencyPeriods(tc.freq)
			if gotN != tc.wantN || gotOK != tc.wantOK {
				t.Errorf("PayoutFrequencyPeriods(%q) = (%d, %v); want (%d, %v)",
					tc.freq, gotN, gotOK, tc.wantN, tc.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration-style cross-function consistency test
// ---------------------------------------------------------------------------

// TestPeriodicVsMaturityConsistency checks that for a 1-year annual payout deposit,
// PeriodicInterest (which pays once at year-end) aligns with AccruedInterest at
// the end of the term -- both representing simple annual interest when n=1.
func TestPeriodicVsAccruedConsistency(t *testing.T) {
	t.Parallel()

	principal := int64(100000) // $1000.00
	rate := 5.0                // 5%

	start := date(2023, 1, 1)
	end := date(2024, 1, 1) // 365 days, non-leap

	periodic := PeriodicInterest(principal, rate, 1) // annual, 1 period/yr
	accrued := AccruedInterest(principal, rate, start, end)

	// Both should equal 5000: 100000 * 0.05 = 5000
	if periodic != 5000 {
		t.Errorf("PeriodicInterest = %d; want 5000", periodic)
	}
	if accrued != 5000 {
		t.Errorf("AccruedInterest at maturity = %d; want 5000", accrued)
	}
	if periodic != accrued {
		t.Errorf("PeriodicInterest (%d) != AccruedInterest at maturity (%d); expected equal for 365-day non-leap year with annual frequency", periodic, accrued)
	}
}
