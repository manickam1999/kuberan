package services

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

// testFXStaleness is the staleness window used by services built in tests that
// do not exercise it.
const testFXStaleness = 168 * time.Hour

var fxNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func mustRate(t *testing.T, s string) fx.Rate {
	t.Helper()
	r, err := fx.ParseRate(s)
	if err != nil {
		t.Fatalf("ParseRate(%q): %v", s, err)
	}
	return r
}

// seedRate inserts one fx_rates row through the service.
func seedRate(t *testing.T, svc FXServicer, base, quote, rate string, at time.Time) {
	t.Helper()
	n, err := svc.RecordRates([]FXRateInput{{Base: base, Quote: quote, Rate: mustRate(t, rate), RecordedAt: at}})
	testutil.AssertNoError(t, err)
	if n != 1 {
		t.Fatalf("seeding %s/%s recorded %d rows, want 1", base, quote, n)
	}
}

func newFXTest(t *testing.T, staleness time.Duration) (FXServicer, *gorm.DB) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	return NewFXService(db, staleness), db
}

func TestFXRate_Identity(t *testing.T) {
	svc, _ := newFXTest(t, time.Hour)
	q, err := svc.Rate("myr", "MYR", fxNow)
	testutil.AssertNoError(t, err)
	if q.Rate != fx.Rate(fx.RateScale) || q.Path != FXPathIdentity || q.Stale || !q.RecordedAt.Equal(fxNow) {
		t.Errorf("unexpected identity quote: %+v", q)
	}
	if q.Base != "MYR" || q.Quote != "MYR" {
		t.Errorf("codes not normalised: %+v", q)
	}
}

func TestFXRate_DirectPicksLatestNotAfterAt(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.40", fxNow.Add(-48*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.45", fxNow.Add(-2*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.99", fxNow.Add(2*time.Hour)) // in the future: ignored

	q, err := svc.Rate("usd", "myr", fxNow)
	testutil.AssertNoError(t, err)
	if q.Rate != mustRate(t, "4.45") || q.Path != FXPathDirect {
		t.Errorf("got %s via %s, want 4.45 direct", q.Rate, q.Path)
	}
	if !q.RecordedAt.Equal(fxNow.Add(-2 * time.Hour)) {
		t.Errorf("RecordedAt = %s", q.RecordedAt)
	}

	// Asking for an earlier time selects the earlier row.
	q, err = svc.Rate("USD", "MYR", fxNow.Add(-24*time.Hour))
	testutil.AssertNoError(t, err)
	if q.Rate != mustRate(t, "4.40") {
		t.Errorf("as-of lookup got %s, want 4.40", q.Rate)
	}

	// Before any observation there is no rate.
	_, err = svc.Rate("USD", "MYR", fxNow.Add(-72*time.Hour))
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
}

func TestFXRate_Inverse(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))

	q, err := svc.Rate("MYR", "USD", fxNow)
	testutil.AssertNoError(t, err)
	if q.Path != FXPathInverse {
		t.Errorf("path = %s, want inverse", q.Path)
	}
	want, err := fx.Invert(mustRate(t, "4.4712"))
	testutil.AssertNoError(t, err)
	if q.Rate != want {
		t.Errorf("rate = %s, want %s", q.Rate, want)
	}
}

func TestFXRate_DirectBeatsNewerInverse(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "MYR", "USD", "0.2000", fxNow.Add(-72*time.Hour)) // older direct
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))    // newer inverse

	q, err := svc.Rate("MYR", "USD", fxNow)
	testutil.AssertNoError(t, err)
	if q.Path != FXPathDirect || q.Rate != mustRate(t, "0.2") {
		t.Errorf("got %s via %s, want 0.2 direct", q.Rate, q.Path)
	}
}

func TestFXRate_Pivot(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	older := fxNow.Add(-30 * time.Hour)
	newer := fxNow.Add(-1 * time.Hour)
	seedRate(t, svc, "EUR", "USD", "1.08", older) // direct leg
	seedRate(t, svc, "USD", "MYR", "4.5", newer)  // direct leg

	q, err := svc.Rate("EUR", "MYR", fxNow)
	testutil.AssertNoError(t, err)
	if q.Path != FXPathPivotUSD || q.Rate != mustRate(t, "4.86") {
		t.Errorf("got %s via %s, want 4.86 pivot:USD", q.Rate, q.Path)
	}
	if !q.RecordedAt.Equal(older) {
		t.Errorf("RecordedAt = %s, want the older leg %s", q.RecordedAt, older)
	}

	// The reverse pivot uses both legs inverted.
	back, err := svc.Rate("MYR", "EUR", fxNow)
	testutil.AssertNoError(t, err)
	if back.Path != FXPathPivotUSD {
		t.Errorf("reverse path = %s", back.Path)
	}
	want, err := fx.Mul(mustInvert(t, mustRate(t, "4.5")), mustInvert(t, mustRate(t, "1.08")))
	testutil.AssertNoError(t, err)
	if back.Rate != want {
		t.Errorf("reverse rate = %s, want %s", back.Rate, want)
	}
}

func mustInvert(t *testing.T, r fx.Rate) fx.Rate {
	t.Helper()
	inv, err := fx.Invert(r)
	testutil.AssertNoError(t, err)
	return inv
}

func TestFXRate_PivotNeedsBothLegs(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "EUR", "USD", "1.08", fxNow.Add(-time.Hour))

	_, err := svc.Rate("EUR", "MYR", fxNow)
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")

	// No pivot is attempted when one side is USD itself.
	_, err = svc.Rate("MYR", "USD", fxNow)
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
}

func TestFXRate_Staleness(t *testing.T) {
	svc, _ := newFXTest(t, 24*time.Hour)
	seedRate(t, svc, "USD", "MYR", "4.4", fxNow.Add(-48*time.Hour))
	seedRate(t, svc, "USD", "SGD", "1.3", fxNow.Add(-24*time.Hour)) // exactly at the limit: fresh
	seedRate(t, svc, "USD", "EUR", "0.9", fxNow.Add(-time.Hour))

	for pair, wantStale := range map[string]bool{"MYR": true, "SGD": false, "EUR": false} {
		q, err := svc.Rate("USD", pair, fxNow)
		testutil.AssertNoError(t, err)
		if q.Stale != wantStale {
			t.Errorf("USD/%s Stale = %v, want %v", pair, q.Stale, wantStale)
		}
	}

	// A pivot is stale when its older leg is.
	seedRate(t, svc, "GBP", "USD", "1.2", fxNow.Add(-72*time.Hour))
	q, err := svc.Rate("GBP", "EUR", fxNow)
	testutil.AssertNoError(t, err)
	if q.Path != FXPathPivotUSD || !q.Stale {
		t.Errorf("pivot quote: %+v, want stale pivot", q)
	}
}

func TestFXRates_BatchMatchesPerPair(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.4712", fxNow.Add(-time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.5000", fxNow.Add(5*time.Hour))
	seedRate(t, svc, "EUR", "USD", "1.08", fxNow.Add(-3*time.Hour))
	seedRate(t, svc, "SGD", "MYR", "3.4", fxNow.Add(-2*time.Hour))
	seedRate(t, svc, "USD", "SGD", "1.31", fxNow.Add(-26*time.Hour))

	pairs := []CurrencyPair{
		{"USD", "MYR"}, {"MYR", "USD"}, {"EUR", "MYR"}, {"MYR", "EUR"}, {"SGD", "MYR"}, {"MYR", "SGD"},
		{"EUR", "SGD"}, {"GBP", "MYR"}, {"MYR", "MYR"}, {"usd", "myr"}, {"USD", "MYR"}, {"", "USD"},
	}
	got, err := svc.Rates(pairs, fxNow)
	testutil.AssertNoError(t, err)

	for _, p := range pairs {
		norm := CurrencyPair{Base: normalizeCode(p.Base), Quote: normalizeCode(p.Quote)}
		single, singleErr := svc.Rate(p.Base, p.Quote, fxNow)
		batch, ok := got[norm]
		if singleErr != nil {
			if ok {
				t.Errorf("%v: batch has a quote but Rate failed: %v", p, singleErr)
			}
			continue
		}
		if !ok {
			t.Errorf("%v: Rate succeeded but the batch has no quote", p)
			continue
		}
		if !reflect.DeepEqual(single, batch) {
			t.Errorf("%v: batch %+v != single %+v", p, batch, single)
		}
	}
	if _, ok := got[CurrencyPair{"GBP", "MYR"}]; ok {
		t.Error("pair without a rate must be absent from the map")
	}
	if _, ok := got[CurrencyPair{"", "USD"}]; ok {
		t.Error("blank pair must be absent")
	}
	if len(got) != 8 {
		t.Errorf("expected 8 resolved pairs, got %d: %v", len(got), got)
	}
}

func TestFXRates_ManyPairsAcrossChunks(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)

	code := func(i int) string {
		return string([]rune{'A' + rune(i/26/26%26), 'A' + rune(i/26%26), 'A' + rune(i%26)})
	}
	const n = 150 // 150 pairs -> 300 legs: more than one lookup chunk
	var inputs []FXRateInput
	var pairs []CurrencyPair
	for i := 0; i < n; i++ {
		base := code(i + 1)
		inputs = append(inputs, FXRateInput{Base: base, Quote: "USD", Rate: mustRate(t, "2"), RecordedAt: fxNow.Add(-time.Hour)})
		pairs = append(pairs, CurrencyPair{base, "USD"}, CurrencyPair{"USD", base})
	}
	_, err := svc.RecordRates(inputs)
	testutil.AssertNoError(t, err)

	got, err := svc.Rates(pairs, fxNow)
	testutil.AssertNoError(t, err)
	if len(got) != len(pairs) {
		t.Fatalf("resolved %d of %d pairs", len(got), len(pairs))
	}
	if q := got[CurrencyPair{"USD", code(1)}]; q == nil || q.Rate != mustRate(t, "0.5") || q.Path != FXPathInverse {
		t.Errorf("unexpected inverse quote: %+v", q)
	}
}

func TestFXRates_Empty(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	got, err := svc.Rates(nil, fxNow)
	testutil.AssertNoError(t, err)
	if got == nil || len(got) != 0 {
		t.Errorf("expected empty non-nil map, got %v", got)
	}
}

func TestFXRecordRates(t *testing.T) {
	t.Run("upsert updates same key", func(t *testing.T) {
		svc, db := newFXTest(t, testFXStaleness)
		at := fxNow.Add(-time.Hour)
		seedRate(t, svc, "USD", "MYR", "4.40", at)

		n, err := svc.RecordRates([]FXRateInput{
			{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.41"), RecordedAt: at, Source: "manual"},
			{Base: "USD", Quote: "SGD", Rate: mustRate(t, "1.3"), RecordedAt: at},
		})
		testutil.AssertNoError(t, err)
		if n != 2 {
			t.Errorf("count = %d, want 2", n)
		}

		var rows []models.FXRate
		testutil.AssertNoError(t, db.Order("quote").Find(&rows).Error)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		myr := rows[0]
		if myr.Quote != "MYR" || myr.Rate != mustRate(t, "4.41") || myr.Source != "manual" {
			t.Errorf("MYR row not updated: %+v", myr)
		}
		if rows[1].Source != "yahoo" {
			t.Errorf("source default = %q, want yahoo", rows[1].Source)
		}
		if myr.ID == "" {
			t.Error("expected a generated ID")
		}
	})

	t.Run("duplicate keys in one batch keep the last", func(t *testing.T) {
		svc, _ := newFXTest(t, testFXStaleness)
		n, err := svc.RecordRates([]FXRateInput{
			{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.40"), RecordedAt: fxNow},
			{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.42"), RecordedAt: fxNow},
		})
		testutil.AssertNoError(t, err)
		if n != 1 {
			t.Errorf("count = %d, want 1", n)
		}
		q, err := svc.Rate("USD", "MYR", fxNow)
		testutil.AssertNoError(t, err)
		if q.Rate != mustRate(t, "4.42") {
			t.Errorf("rate = %s, want 4.42", q.Rate)
		}
	})

	t.Run("non-UTC time is stored as the same instant", func(t *testing.T) {
		svc, _ := newFXTest(t, testFXStaleness)
		kl := time.FixedZone("MYT", 8*3600)
		seedRate(t, svc, "USD", "MYR", "4.4", time.Date(2026, 11, 3, 18, 0, 0, 0, kl)) // 10:00 UTC
		q, err := svc.Rate("USD", "MYR", time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC))
		testutil.AssertNoError(t, err)
		if !q.RecordedAt.Equal(time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)) || q.RecordedAt.Location() != time.UTC {
			t.Errorf("RecordedAt = %s", q.RecordedAt)
		}
	})

	t.Run("more rows than one insert batch", func(t *testing.T) {
		svc, _ := newFXTest(t, testFXStaleness)
		inputs := make([]FXRateInput, 0, fxInsertBatchSize+25)
		for i := 0; i < fxInsertBatchSize+25; i++ {
			inputs = append(inputs, FXRateInput{Base: "USD", Quote: "MYR", Rate: mustRate(t, "4.4"), RecordedAt: fxNow.Add(-time.Duration(i) * time.Minute)})
		}
		n, err := svc.RecordRates(inputs)
		testutil.AssertNoError(t, err)
		if n != len(inputs) {
			t.Errorf("count = %d, want %d", n, len(inputs))
		}
	})

	good := FXRateInput{Base: "USD", Quote: "MYR", Rate: fx.Rate(fx.RateScale), RecordedAt: fxNow}
	bad := func(mutate func(*FXRateInput)) FXRateInput {
		in := good
		mutate(&in)
		return in
	}
	invalid := map[string]FXRateInput{
		"lowercase base": bad(func(in *FXRateInput) { in.Base = "usd" }),
		"short base":     bad(func(in *FXRateInput) { in.Base = "US" }),
		"digit quote":    bad(func(in *FXRateInput) { in.Quote = "M1R" }),
		"same pair":      bad(func(in *FXRateInput) { in.Quote = "USD" }),
		"zero rate":      bad(func(in *FXRateInput) { in.Rate = 0 }),
		"negative rate":  bad(func(in *FXRateInput) { in.Rate = -1 }),
		"huge rate":      bad(func(in *FXRateInput) { in.Rate = fx.MaxRate + 1 }),
		"zero time":      bad(func(in *FXRateInput) { in.RecordedAt = time.Time{} }),
		"long source":    bad(func(in *FXRateInput) { in.Source = "0123456789012345678901234567890123" }),
	}
	for name, in := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			svc, db := newFXTest(t, testFXStaleness)
			_, err := svc.RecordRates([]FXRateInput{good, in})
			testutil.AssertAppError(t, err, "INVALID_INPUT")
			if want := "rates[1]"; !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %s", err.Error(), want)
			}
			var count int64
			testutil.AssertNoError(t, db.Model(&models.FXRate{}).Count(&count).Error)
			if count != 0 {
				t.Errorf("a failed batch stored %d rows", count)
			}
		})
	}

	t.Run("empty", func(t *testing.T) {
		svc, _ := newFXTest(t, testFXStaleness)
		_, err := svc.RecordRates(nil)
		testutil.AssertAppError(t, err, "INVALID_INPUT")
	})
}

func TestFXListRates(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	seedRate(t, svc, "USD", "MYR", "4.43", fxNow.Add(-1*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.41", fxNow.Add(-3*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.42", fxNow.Add(-2*time.Hour))
	seedRate(t, svc, "USD", "MYR", "4.00", fxNow.Add(-100*time.Hour)) // outside the range
	seedRate(t, svc, "MYR", "USD", "0.22", fxNow.Add(-2*time.Hour))   // inverse pair: not listed

	got, err := svc.ListRates("usd", "myr", fxNow.Add(-4*time.Hour), fxNow)
	testutil.AssertNoError(t, err)
	if len(got) != 3 {
		t.Fatalf("expected 3 rates, got %d", len(got))
	}
	for i, want := range []string{"4.41", "4.42", "4.43"} {
		if got[i].Rate != mustRate(t, want) {
			t.Errorf("rates[%d] = %s, want %s (ascending order)", i, got[i].Rate, want)
		}
	}

	none, err := svc.ListRates("USD", "SGD", fxNow.Add(-4*time.Hour), fxNow)
	testutil.AssertNoError(t, err)
	if none == nil || len(none) != 0 {
		t.Errorf("expected an empty non-nil slice, got %v", none)
	}
}

func TestFXNeededPairs(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		svc, _ := newFXTest(t, testFXStaleness)
		pairs, bases, err := svc.NeededPairs()
		testutil.AssertNoError(t, err)
		if pairs == nil || bases == nil || len(pairs) != 0 || len(bases) != 0 {
			t.Errorf("expected empty non-nil results, got %v %v", pairs, bases)
		}
	})

	t.Run("unions currencies with every base plus base to USD", func(t *testing.T) {
		svc, db := newFXTest(t, testFXStaleness)

		u1 := testutil.CreateTestUser(t, db) // MYR by default
		u2 := testutil.CreateTestUser(t, db)
		testutil.AssertNoError(t, db.Model(u2).Update("base_currency", "SGD").Error)
		u3 := testutil.CreateTestUser(t, db) // inactive: its base is ignored
		testutil.AssertNoError(t, db.Model(u3).Updates(map[string]interface{}{"base_currency": "GBP", "is_active": false}).Error)

		addAccount := func(currency string, active, deleted bool) {
			acc := testutil.CreateTestCashAccount(t, db, u1.ID)
			testutil.AssertNoError(t, db.Model(acc).Updates(map[string]interface{}{"currency": currency, "is_active": active}).Error)
			if deleted {
				testutil.AssertNoError(t, db.Delete(acc).Error)
			}
		}
		addAccount("eur", true, false)  // active; lower-case stored value is upper-cased
		addAccount("MYR", true, false)  // equals a base
		addAccount("JPY", false, false) // inactive: excluded
		addAccount("CHF", true, true)   // deleted: excluded
		addAccount("USD", true, false)

		sec := testutil.CreateTestSecurity(t, db)
		testutil.AssertNoError(t, db.Model(sec).Update("currency", "HKD").Error)
		gone := testutil.CreateTestSecurityWithParams(t, db, "GONE", "Gone", models.AssetTypeStock, "X")
		testutil.AssertNoError(t, db.Model(gone).Update("currency", "AUD").Error)
		testutil.AssertNoError(t, db.Delete(gone).Error)

		pairs, bases, err := svc.NeededPairs()
		testutil.AssertNoError(t, err)

		if !reflect.DeepEqual(bases, []string{"MYR", "SGD"}) {
			t.Errorf("baseCurrencies = %v, want [MYR SGD]", bases)
		}
		// Held currencies (EUR, HKD, MYR, USD) against each base (MYR, SGD), minus
		// identities, plus each base against USD. SGD is not held, so (SGD, MYR)
		// is absent.
		want := []CurrencyPair{
			{"EUR", "MYR"}, {"EUR", "SGD"},
			{"HKD", "MYR"}, {"HKD", "SGD"},
			{"MYR", "SGD"}, {"MYR", "USD"},
			{"SGD", "USD"},
			{"USD", "MYR"}, {"USD", "SGD"},
		}
		if !reflect.DeepEqual(pairs, want) {
			t.Errorf("pairs = %v\nwant   %v", pairs, want)
		}
	})

	t.Run("USD base needs no USD pair", func(t *testing.T) {
		svc, db := newFXTest(t, testFXStaleness)
		u := testutil.CreateTestUser(t, db)
		testutil.AssertNoError(t, db.Model(u).Update("base_currency", "USD").Error)
		acc := testutil.CreateTestCashAccount(t, db, u.ID)
		testutil.AssertNoError(t, db.Model(acc).Update("currency", "EUR").Error)

		pairs, bases, err := svc.NeededPairs()
		testutil.AssertNoError(t, err)
		if !reflect.DeepEqual(pairs, []CurrencyPair{{"EUR", "USD"}}) || !reflect.DeepEqual(bases, []string{"USD"}) {
			t.Errorf("pairs = %v, bases = %v", pairs, bases)
		}
	})
}

func TestFXIsSupportedCurrency(t *testing.T) {
	svc, _ := newFXTest(t, time.Hour)
	seedRate(t, svc, "USD", "MYR", "4.4", fxNow.Add(-1000*time.Hour)) // very stale: still counts
	seedRate(t, svc, "JPY", "USD", "0.0066", fxNow.Add(-time.Hour))
	seedRate(t, svc, "EUR", "USD", "1.08", fxNow.Add(-time.Hour))

	tests := []struct {
		code string
		want bool
	}{
		{"USD", true},
		{"usd", true},
		{"MYR", true},  // inverse of USD/MYR
		{"EUR", true},  // direct EUR/USD
		{"SGD", false}, // no rate
		{"JPY", false}, // not a 2-decimal currency, even with a rate
		{"KWD", false},
		{"US", false},
		{"USDX", false},
		{"U5D", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := svc.IsSupportedCurrency(tt.code, fxNow); got != tt.want {
			t.Errorf("IsSupportedCurrency(%q) = %v, want %v", tt.code, got, tt.want)
		}
	}
	// A rate recorded after `at` does not count.
	if svc.IsSupportedCurrency("EUR", fxNow.Add(-24*time.Hour)) {
		t.Error("EUR should be unsupported before its first rate")
	}
}

func TestFXQuoteErrorIsAppError(t *testing.T) {
	svc, _ := newFXTest(t, testFXStaleness)
	_, err := svc.Rate("USD", "MYR", fxNow)
	if err == nil || fmt.Sprint(err) != apperrors.ErrFXRateUnavailable.Message {
		t.Errorf("unexpected error: %v", err)
	}
}

// Many rows per leg: the latest row <= at wins, an older-than-at row is not
// preferred over it, and rows after at are ignored (the per-leg LIMIT 1 shape).
func TestFXRate_ManyRowsPerLegPicksLatestNotAfterAt(t *testing.T) {
	svc, db := newFXTest(t, testFXStaleness)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]models.FXRate, 0, 600)
	for i := 0; i < 600; i++ {
		rows = append(rows, models.FXRate{
			Base: "USD", Quote: "MYR", Rate: fx.Rate(40_000_000_000 + int64(i)),
			RecordedAt: start.Add(time.Duration(i) * 30 * time.Minute), Source: "test",
		}, models.FXRate{
			Base: "EUR", Quote: "MYR", Rate: fx.Rate(50_000_000_000 + int64(i)),
			RecordedAt: start.Add(time.Duration(i) * 30 * time.Minute), Source: "test",
		})
	}
	testutil.AssertNoError(t, db.CreateInBatches(&rows, 100).Error)

	at := start.Add(100*30*time.Minute + 7*time.Minute) // between row 100 and 101
	got, err := svc.Rates([]CurrencyPair{{"USD", "MYR"}, {"EUR", "MYR"}}, at)
	testutil.AssertNoError(t, err)
	if q := got[CurrencyPair{"USD", "MYR"}]; q == nil || q.Rate != fx.Rate(40_000_000_000+100) || !q.RecordedAt.Equal(start.Add(100*30*time.Minute)) {
		t.Errorf("USD/MYR = %+v, want row 100", q)
	}
	if q := got[CurrencyPair{"EUR", "MYR"}]; q == nil || q.Rate != fx.Rate(50_000_000_000+100) {
		t.Errorf("EUR/MYR = %+v, want row 100", q)
	}

	// Exactly at a row's time, that row is included.
	exact := start.Add(250 * 30 * time.Minute)
	got, err = svc.Rates([]CurrencyPair{{"USD", "MYR"}}, exact)
	testutil.AssertNoError(t, err)
	if q := got[CurrencyPair{"USD", "MYR"}]; q == nil || q.Rate != fx.Rate(40_000_000_000+250) {
		t.Errorf("USD/MYR at exact = %+v, want row 250", q)
	}

	// Before the first row there is no rate.
	got, err = svc.Rates([]CurrencyPair{{"USD", "MYR"}}, start.Add(-time.Minute))
	testutil.AssertNoError(t, err)
	if got[CurrencyPair{"USD", "MYR"}] != nil {
		t.Errorf("rate before first row = %+v, want none", got[CurrencyPair{"USD", "MYR"}])
	}
}
