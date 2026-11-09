package services

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/testutil"
)

var jan1 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func usdMYR() CurrencyPair { return CurrencyPair{Base: "USD", Quote: "MYR"} }

// stmtCounter is a GORM logger that counts executed statements.
type stmtCounter struct {
	logger.Interface
	n atomic.Int64
}

func (c *stmtCounter) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	c.n.Add(1)
}

func (c *stmtCounter) LogMode(logger.LogLevel) logger.Interface { return c }

// newCountingFX returns an FX service whose every SQL statement is counted.
func newCountingFX(t *testing.T) (FXServicer, *stmtCounter) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	c := &stmtCounter{Interface: logger.Discard}
	return NewFXService(db.Session(&gorm.Session{Logger: c}), testFXStaleness), c
}

func TestRatesByDay_CarryForwardOverGap(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.0", jan1.Add(10*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.5", jan1.AddDate(0, 0, 3).Add(10*time.Hour))

	got, err := svc.RatesByDay([]CurrencyPair{usdMYR()}, jan1, jan1.AddDate(0, 0, 4))
	testutil.AssertNoError(t, err)
	want := map[string]string{
		"2026-01-01": "4.0", "2026-01-02": "4.0", "2026-01-03": "4.0", "2026-01-04": "4.5", "2026-01-05": "4.5",
	}
	for day, r := range want {
		q := got[usdMYR()][day]
		if q == nil || q.Rate != mustRate(t, r) || q.Path != FXPathDirect {
			t.Errorf("%s = %+v, want %s direct", day, q, r)
		}
	}
	if len(got[usdMYR()]) != 5 {
		t.Errorf("got %d days, want 5", len(got[usdMYR()]))
	}
}

func TestRatesByDay_CarryInFromBeforeStart(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.2", jan1.AddDate(0, 0, -10))

	got, err := svc.RatesByDay([]CurrencyPair{usdMYR()}, jan1, jan1.AddDate(0, 0, 1))
	testutil.AssertNoError(t, err)
	for _, day := range []string{"2026-01-01", "2026-01-02"} {
		if q := got[usdMYR()][day]; q == nil || q.Rate != mustRate(t, "4.2") {
			t.Errorf("%s = %+v, want carried-in 4.2", day, q)
		}
	}
}

func TestRatesByDay_DaysBeforeFirstRateAreMissing(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.2", jan1.AddDate(0, 0, 2).Add(8*time.Hour))

	got, err := svc.RatesByDay([]CurrencyPair{usdMYR()}, jan1, jan1.AddDate(0, 0, 3))
	testutil.AssertNoError(t, err)
	days := got[usdMYR()]
	if days["2026-01-01"] != nil || days["2026-01-02"] != nil {
		t.Errorf("days before the first rate must be absent: %+v", days)
	}
	if days["2026-01-03"] == nil || days["2026-01-04"] == nil {
		t.Errorf("days from the first rate must resolve: %+v", days)
	}
}

func TestRatesByDay_NoRowsAtAll(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	got, err := svc.RatesByDay([]CurrencyPair{usdMYR()}, jan1, jan1.AddDate(0, 0, 2))
	testutil.AssertNoError(t, err)
	if len(got[usdMYR()]) != 0 {
		t.Errorf("expected no quotes, got %+v", got[usdMYR()])
	}
}

func TestRatesByDay_InverseAndPivotPerDay(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	// MYR->USD stored; USD->MYR is its inverse. The rate changes on day 2.
	seedRate(t, svc, "MYR", "USD", "0.25", jan1.Add(time.Hour))
	seedRate(t, svc, "MYR", "USD", "0.20", jan1.AddDate(0, 0, 1).Add(time.Hour))
	// EUR->MYR has no direct row: pivot EUR->USD x USD->MYR.
	seedRate(t, svc, "EUR", "USD", "1.1", jan1.Add(time.Hour))
	seedRate(t, svc, "EUR", "USD", "1.2", jan1.AddDate(0, 0, 1).Add(time.Hour))
	eurMYR := CurrencyPair{Base: "EUR", Quote: "MYR"}

	got, err := svc.RatesByDay([]CurrencyPair{usdMYR(), eurMYR}, jan1, jan1.AddDate(0, 0, 1))
	testutil.AssertNoError(t, err)

	if q := got[usdMYR()]["2026-01-01"]; q == nil || q.Rate != mustRate(t, "4") || q.Path != FXPathInverse {
		t.Errorf("day1 usd/myr = %+v, want 4 inverse", q)
	}
	if q := got[usdMYR()]["2026-01-02"]; q == nil || q.Rate != mustRate(t, "5") || q.Path != FXPathInverse {
		t.Errorf("day2 usd/myr = %+v, want 5 inverse", q)
	}
	// Day 1: 1.1 * 4 = 4.4; day 2: 1.2 * 5 = 6.
	if q := got[eurMYR]["2026-01-01"]; q == nil || q.Rate != mustRate(t, "4.4") || q.Path != FXPathPivotUSD {
		t.Errorf("day1 eur/myr = %+v, want 4.4 pivot", q)
	}
	if q := got[eurMYR]["2026-01-02"]; q == nil || q.Rate != mustRate(t, "6") || q.Path != FXPathPivotUSD {
		t.Errorf("day2 eur/myr = %+v, want 6 pivot", q)
	}
}

func TestRatesByDay_IdentityAndStalenessAtEndOfDay(t *testing.T) {
	svc, _ := newFXTest(t, 24*time.Hour)
	seedRate(t, svc, "USD", "MYR", "4.0", jan1.Add(12*time.Hour))
	myr := CurrencyPair{Base: "myr", Quote: "MYR"}

	got, err := svc.RatesByDay([]CurrencyPair{usdMYR(), myr}, jan1, jan1.AddDate(0, 0, 2))
	testutil.AssertNoError(t, err)
	if q := got[CurrencyPair{"MYR", "MYR"}]["2026-01-03"]; q == nil || q.Path != FXPathIdentity {
		t.Errorf("identity quote missing: %+v", q)
	}
	// Recorded 12:00 day 1; end of day 1 is 12h later (fresh), end of day 2 is 36h (stale).
	if q := got[usdMYR()]["2026-01-01"]; q == nil || q.Stale {
		t.Errorf("day1 must be fresh: %+v", q)
	}
	if q := got[usdMYR()]["2026-01-02"]; q == nil || !q.Stale {
		t.Errorf("day2 must be stale: %+v", q)
	}
}

func TestRatesByDay_EmptyAndReversedRange(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	got, err := svc.RatesByDay([]CurrencyPair{usdMYR()}, jan1.AddDate(0, 0, 2), jan1)
	testutil.AssertNoError(t, err)
	if len(got) != 0 {
		t.Errorf("reversed range must be empty: %+v", got)
	}
}

func TestRatesByDay_MatchesPerDayRate(t *testing.T) {
	svc, _ := newFXTest(t, 36*time.Hour)
	rng := rand.New(rand.NewSource(42))
	span := 60
	legs := [][2]string{{"USD", "MYR"}, {"MYR", "SGD"}, {"EUR", "USD"}, {"GBP", "USD"}, {"USD", "SGD"}}
	for _, l := range legs {
		at := jan1.AddDate(0, 0, -rng.Intn(5))
		for at.Before(jan1.AddDate(0, 0, span)) {
			cents := 100 + rng.Intn(900)
			seedRate(t, svc, l[0], l[1], fx.Rate(int64(cents)*fx.RateScale/100).String(), at)
			at = at.Add(time.Duration(1+rng.Intn(100)) * time.Hour)
		}
	}
	pairs := []CurrencyPair{usdMYR(), {"SGD", "MYR"}, {"EUR", "MYR"}, {"GBP", "SGD"}, {"MYR", "USD"}, {"EUR", "SGD"}}
	end := jan1.AddDate(0, 0, span-1)
	batch, err := svc.RatesByDay(pairs, jan1, end)
	testutil.AssertNoError(t, err)

	for i := 0; i < 80; i++ {
		day := jan1.AddDate(0, 0, rng.Intn(span))
		for _, p := range pairs {
			want, rerr := svc.Rate(p.Base, p.Quote, endOfDay(day))
			got := batch[p][day.Format(dayLayout)]
			if rerr != nil {
				if !errors.Is(rerr, apperrors.ErrFXRateUnavailable) || got != nil {
					t.Fatalf("%v %s: Rate err %v but batch %+v", p, day.Format(dayLayout), rerr, got)
				}
				continue
			}
			if got == nil || !reflect.DeepEqual(want, got) {
				t.Fatalf("%v %s: batch %+v != Rate %+v", p, day.Format(dayLayout), got, want)
			}
		}
	}
}

func TestRatesByDay_QueryCountBoundedFor366Days(t *testing.T) {
	svc, counter := newCountingFX(t)
	// A rate every day for two legs (plus the USD pivot legs).
	var rows []FXRateInput
	for d := 0; d < 366; d++ {
		at := jan1.AddDate(0, 0, d).Add(9 * time.Hour)
		rows = append(rows,
			FXRateInput{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.4"), RecordedAt: at},
			FXRateInput{Base: "EUR", Quote: "USD", Rate: mustRate(t, "1.1"), RecordedAt: at},
			FXRateInput{Base: "SGD", Quote: "USD", Rate: mustRate(t, "0.75"), RecordedAt: at})
	}
	if _, err := svc.RecordRates(rows); err != nil {
		t.Fatal(err)
	}

	counter.n.Store(0)
	got, err := svc.RatesByDay([]CurrencyPair{usdMYR(), {"EUR", "MYR"}, {"SGD", "MYR"}}, jan1, jan1.AddDate(0, 0, 365))
	testutil.AssertNoError(t, err)
	n := counter.n.Load()
	t.Logf("RatesByDay over 366 days, 3 pairs: %d statements", n)
	if n > 3 {
		t.Errorf("statements = %d, want <= 3 (carry-in + range)", n)
	}
	if len(got[CurrencyPair{"EUR", "MYR"}]) != 366 {
		t.Errorf("EUR/MYR days = %d, want 366", len(got[CurrencyPair{"EUR", "MYR"}]))
	}
}

// panicFXDated fails the test if any FX method is reached.
type panicFXDated struct{ FXServicer }

func (panicFXDated) RatesByDay([]CurrencyPair, time.Time, time.Time) (map[CurrencyPair]map[string]*FXQuote, error) {
	panic("RatesByDay must not be called on the fast path")
}

func TestDatedValuer_FastPathMakesNoFXQuery(t *testing.T) {
	for name, fxs := range map[string]FXServicer{"nil fx": nil, "panicking fx": panicFXDated{}} {
		v, err := newDatedValuer(fxs, "usd", jan1, jan1.AddDate(0, 0, 5), []string{"USD", "usd", ""})
		testutil.AssertNoError(t, err)
		if got, ok := v.Convert(1234, "USD", "2026-01-02"); !ok || got != 1234 {
			t.Errorf("%s: identity = %d %v", name, got, ok)
		}
	}
	// Nil FX with a foreign currency: missing, never 1:1.
	v, err := newDatedValuer(nil, "MYR", jan1, jan1, []string{"USD"})
	testutil.AssertNoError(t, err)
	if _, ok := v.Convert(100, "USD", "2026-01-01"); ok {
		t.Error("nil FX must not convert a foreign currency")
	}
}

func TestDatedValuer_ConvertsAtTheDaysRate(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.0", jan1.Add(time.Hour))
	seedRate(t, svc, "USD", "MYR", "5.0", jan1.AddDate(0, 0, 2).Add(time.Hour))
	v, err := newDatedValuer(svc, "MYR", jan1, jan1.AddDate(0, 0, 3), []string{"USD", "GBP"})
	testutil.AssertNoError(t, err)

	cases := map[string]int64{"2026-01-01": 4000, "2026-01-02": 4000, "2026-01-03": 5000, "2026-01-04": 5000}
	for day, want := range cases {
		if got, ok := v.Convert(1000, "USD", day); !ok || got != want {
			t.Errorf("%s = %d %v, want %d", day, got, ok, want)
		}
	}
	if _, ok := v.Convert(1000, "GBP", "2026-01-01"); ok {
		t.Error("GBP has no rate: must not convert")
	}
	if _, ok := v.Convert(1000, "USD", "2025-12-31"); ok {
		t.Error("a day outside the fetched range must not convert")
	}
}
