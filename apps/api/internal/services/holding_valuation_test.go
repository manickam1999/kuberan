package services

import (
	"testing"
	"time"

	"kuberan/internal/testutil"
)

// panicFX fails the test if any FX lookup happens (nil embedded interface).
type panicFX struct{ FXServicer }

// countingFX counts Rates calls.
type countingFX struct {
	FXServicer
	calls int
}

func (c *countingFX) Rates(pairs []CurrencyPair, at time.Time) (map[CurrencyPair]*FXQuote, error) {
	c.calls++
	return c.FXServicer.Rates(pairs, at)
}

func TestValuer_IdentityNeedsNoFXQuery(t *testing.T) {
	for name, fxs := range map[string]FXServicer{"nil fx": nil, "panicking fx": panicFX{}} {
		t.Run(name, func(t *testing.T) {
			v, err := newValuer(fxs, "myr", fxNow, []string{"MYR", "myr"})
			testutil.AssertNoError(t, err)
			got, quote, ok := v.Convert(12345, "MYR")
			if !ok || got != 12345 || quote != nil {
				t.Errorf("identity = %d %v %v", got, quote, ok)
			}
		})
	}
}

func TestValuer_NilFXOnlyIdentity(t *testing.T) {
	v, err := newValuer(nil, "MYR", fxNow, []string{"USD"})
	testutil.AssertNoError(t, err)
	if _, _, ok := v.Convert(100, "USD"); ok {
		t.Error("USD must not convert without an FX service")
	}
}

func TestValuer_DirectInverseMissing(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))
	seedRate(t, svc, "MYR", "EUR", "0.2", fxNow.Add(-time.Hour))

	v, err := newValuer(svc, "MYR", fxNow, []string{"USD", "EUR", "GBP"})
	testutil.AssertNoError(t, err)

	got, q, ok := v.Convert(10000, "USD")
	if !ok || got != 44712 || q.Path != FXPathDirect {
		t.Errorf("direct = %d %+v %v, want 44712 direct", got, q, ok)
	}

	// EUR->MYR exists only as the inverse of MYR->EUR (1/0.2 = 5).
	got, q, ok = v.Convert(10000, "EUR")
	if !ok || got != 50000 || q.Path != FXPathInverse {
		t.Errorf("inverse = %d %+v %v, want 50000 inverse", got, q, ok)
	}

	if _, _, ok := v.Convert(10000, "GBP"); ok {
		t.Error("GBP has no rate: ok must be false")
	}
}

func TestValuer_UsesOneRatesCall(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))
	seedRate(t, svc, "EUR", "MYR", "4.9", fxNow.Add(-time.Hour))
	spy := &countingFX{FXServicer: svc}

	_, err := newValuer(spy, "MYR", fxNow, []string{"USD", "EUR", "MYR", "USD"})
	testutil.AssertNoError(t, err)
	if spy.calls != 1 {
		t.Errorf("Rates calls = %d, want 1", spy.calls)
	}
}

func TestValuer_LineValueRoundedOnce(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))
	v, err := newValuer(svc, "MYR", fxNow, []string{"USD"})
	testutil.AssertNoError(t, err)

	// 3 x 33.33 USD. Line first: 9999 * 4.4712 = 44707.5288 -> 44708.
	line, _, ok := v.Convert(valueCents(3, 3333), "USD")
	if !ok || line != 44708 {
		t.Fatalf("line-first = %d (ok=%v), want 44708", line, ok)
	}
	// Unit price first would round 3333 * 4.4712 = 14902.5096 -> 14903, x3 = 44709.
	unit, _, _ := v.Convert(3333, "USD")
	if unitFirst := valueCents(3, unit); unitFirst != 44709 {
		t.Fatalf("unit-first = %d, want 44709", unitFirst)
	}
}

func TestValuer_StalePropagates(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-2*testFXStaleness))
	seedRate(t, svc, "EUR", "MYR", "4.9", fxNow.Add(-time.Hour))
	v, err := newValuer(svc, "MYR", fxNow, []string{"USD", "EUR"})
	testutil.AssertNoError(t, err)

	if _, q, ok := v.Convert(100, "USD"); !ok || !q.Stale {
		t.Errorf("USD quote must be stale: %+v", q)
	}
	if _, q, ok := v.Convert(100, "EUR"); !ok || q.Stale {
		t.Errorf("EUR quote must be fresh: %+v", q)
	}
}

func TestValuer_ConvertOverflowIsNotOK(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "99999999", fxNow.Add(-time.Hour))
	v, err := newValuer(svc, "MYR", fxNow, []string{"USD"})
	testutil.AssertNoError(t, err)
	if _, _, ok := v.Convert(int64(1)<<62, "USD"); ok {
		t.Error("overflow must report ok=false")
	}
}
