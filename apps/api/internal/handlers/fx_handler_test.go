package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// --- mock FX service ---

type mockFXService struct {
	rateFn        func(from, to string, at time.Time) (*services.FXQuote, error)
	recordRatesFn func(rates []services.FXRateInput) (int, error)
	listRatesFn   func(base, quote string, from, to time.Time) ([]models.FXRate, error)
	neededPairsFn func() ([]services.CurrencyPair, []string, error)
}

var _ services.FXServicer = (*mockFXService)(nil)

func (m *mockFXService) Rate(from, to string, at time.Time) (*services.FXQuote, error) {
	if m.rateFn != nil {
		return m.rateFn(from, to, at)
	}
	return nil, apperrors.ErrFXRateUnavailable
}

func (m *mockFXService) Rates(_ []services.CurrencyPair, _ time.Time) (map[services.CurrencyPair]*services.FXQuote, error) {
	return map[services.CurrencyPair]*services.FXQuote{}, nil
}

func (m *mockFXService) RecordRates(rates []services.FXRateInput) (int, error) {
	if m.recordRatesFn != nil {
		return m.recordRatesFn(rates)
	}
	return len(rates), nil
}

func (m *mockFXService) ListRates(base, quote string, from, to time.Time) ([]models.FXRate, error) {
	if m.listRatesFn != nil {
		return m.listRatesFn(base, quote, from, to)
	}
	return []models.FXRate{}, nil
}

func (m *mockFXService) NeededPairs() ([]services.CurrencyPair, []string, error) {
	if m.neededPairsFn != nil {
		return m.neededPairsFn()
	}
	return nil, nil, nil
}

func (m *mockFXService) RatesByDay(_ []services.CurrencyPair, _, _ time.Time) (map[services.CurrencyPair]map[string]*services.FXQuote, error) {
	return map[services.CurrencyPair]map[string]*services.FXQuote{}, nil
}

func (m *mockFXService) IsSupportedCurrency(_ string, _ time.Time) bool { return true }

func setupFXRouter(h *FXHandler) *gin.Engine {
	r := gin.New()
	r.POST("/pipeline/fx-rates", h.RecordRates)
	r.GET("/pipeline/fx-pairs", h.GetPairs)
	r.GET("/fx-rates", h.ListRates)
	r.GET("/fx-rates/latest", h.LatestRate)
	return r
}

func mustFXRate(t *testing.T, s string) fx.Rate {
	t.Helper()
	r, err := fx.ParseRate(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFXHandler_RecordRates(t *testing.T) {
	const okBody = `{"rates":[{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":"2026-11-03T08:00:00Z","source":"yahoo"}]}`

	t.Run("records parsed rates and audits", func(t *testing.T) {
		var got []services.FXRateInput
		svc := &mockFXService{recordRatesFn: func(rates []services.FXRateInput) (int, error) {
			got = rates
			return len(rates), nil
		}}
		audit := &recordingAudit{}
		r := setupFXRouter(NewFXHandler(svc, audit))

		rec := doRequest(r, "POST", "/pipeline/fx-rates", okBody)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if v := parseJSON(t, rec)["recorded"]; v != float64(1) {
			t.Errorf("expected recorded=1, got %v", v)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 input, got %d", len(got))
		}
		in := got[0]
		if in.Base != "USD" || in.Quote != "MYR" || in.Rate != 44_712_000_000 || in.Source != "yahoo" ||
			!in.RecordedAt.Equal(time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)) {
			t.Errorf("unexpected input %+v", in)
		}
		if e := audit.only(t); e.action != "PIPELINE_RECORD_FX_RATES" {
			t.Errorf("unexpected audit entry %+v", e)
		}
	})

	t.Run("source is optional", func(t *testing.T) {
		var got []services.FXRateInput
		svc := &mockFXService{recordRatesFn: func(rates []services.FXRateInput) (int, error) {
			got = rates
			return 1, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		rec := doRequest(r, "POST", "/pipeline/fx-rates",
			`{"rates":[{"base":"USD","quote":"MYR","rate":"4","recorded_at":"2026-11-03T08:00:00Z"}]}`)

		if rec.Code != http.StatusOK || len(got) != 1 || got[0].Source != "" {
			t.Fatalf("expected 200 with empty source, got %d %+v", rec.Code, got)
		}
	})

	t.Run("a bad rate string names its index", func(t *testing.T) {
		called := false
		svc := &mockFXService{recordRatesFn: func([]services.FXRateInput) (int, error) {
			called = true
			return 0, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		body := `{"rates":[` +
			`{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":"2026-11-03T08:00:00Z"},` +
			`{"base":"USD","quote":"SGD","rate":"1e3","recorded_at":"2026-11-03T08:00:00Z"}]}`
		rec := doRequest(r, "POST", "/pipeline/fx-rates", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "rates[1].rate") {
			t.Errorf("error does not name rates[1].rate: %s", rec.Body.String())
		}
		if called {
			t.Error("service must not be called")
		}
	})

	bad := map[string]string{
		"rate as a JSON number": `{"rates":[{"base":"USD","quote":"MYR","rate":4.4712,"recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"missing rate":          `{"rates":[{"base":"USD","quote":"MYR","recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"zero rate":             `{"rates":[{"base":"USD","quote":"MYR","rate":"0","recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"too many decimals":     `{"rates":[{"base":"USD","quote":"MYR","rate":"1.12345678901","recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"missing base":          `{"rates":[{"quote":"MYR","rate":"4","recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"non-ISO quote":         `{"rates":[{"base":"USD","quote":"ZZZ","rate":"4","recorded_at":"2026-11-03T08:00:00Z"}]}`,
		"missing recorded_at":   `{"rates":[{"base":"USD","quote":"MYR","rate":"4"}]}`,
		"bad recorded_at":       `{"rates":[{"base":"USD","quote":"MYR","rate":"4","recorded_at":"yesterday"}]}`,
		"long source":           `{"rates":[{"base":"USD","quote":"MYR","rate":"4","recorded_at":"2026-11-03T08:00:00Z","source":"0123456789012345678901234567890123"}]}`,
		"empty rates":           `{"rates":[]}`,
		"no rates":              `{}`,
	}
	for name, body := range bad {
		t.Run("rejects "+name, func(t *testing.T) {
			r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
			rec := doRequest(r, "POST", "/pipeline/fx-rates", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
		})
	}

	t.Run("rejects more than 5000 rates", func(t *testing.T) {
		entry := `{"base":"USD","quote":"MYR","rate":"4","recorded_at":"2026-11-03T08:00:00Z"}`
		body := `{"rates":[` + strings.Repeat(entry+",", 5000) + entry + `]}`
		r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
		if rec := doRequest(r, "POST", "/pipeline/fx-rates", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("propagates service errors", func(t *testing.T) {
		svc := &mockFXService{recordRatesFn: func([]services.FXRateInput) (int, error) {
			return 0, apperrors.WithMessage(apperrors.ErrInvalidInput, "rates[0]: base and quote must differ")
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))
		rec := doRequest(r, "POST", "/pipeline/fx-rates", okBody)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestFXHandler_GetPairs(t *testing.T) {
	t.Run("returns pairs and base currencies", func(t *testing.T) {
		svc := &mockFXService{neededPairsFn: func() ([]services.CurrencyPair, []string, error) {
			return []services.CurrencyPair{{Base: "USD", Quote: "MYR"}, {Base: "EUR", Quote: "MYR"}}, []string{"MYR"}, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		rec := doRequest(r, "GET", "/pipeline/fx-pairs", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := parseJSON(t, rec)
		pairs := body["pairs"].([]interface{})
		if len(pairs) != 2 || pairs[0].(map[string]interface{})["base"] != "USD" || pairs[0].(map[string]interface{})["quote"] != "MYR" {
			t.Errorf("unexpected pairs %v", pairs)
		}
		if bases := body["base_currencies"].([]interface{}); len(bases) != 1 || bases[0] != "MYR" {
			t.Errorf("unexpected base currencies %v", bases)
		}
	})

	t.Run("pairs is never null", func(t *testing.T) {
		r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{})) // service returns nil slices

		rec := doRequest(r, "GET", "/pipeline/fx-pairs", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if got := rec.Body.String(); !strings.Contains(got, `"pairs":[]`) || !strings.Contains(got, `"base_currencies":[]`) {
			t.Errorf("expected empty arrays, got %s", got)
		}
	})

	t.Run("propagates service errors", func(t *testing.T) {
		svc := &mockFXService{neededPairsFn: func() ([]services.CurrencyPair, []string, error) {
			return nil, nil, apperrors.ErrInternalServer
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))
		if rec := doRequest(r, "GET", "/pipeline/fx-pairs", ""); rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestFXHandler_ListRates(t *testing.T) {
	t.Run("returns rates as decimal strings and defaults to 30 days", func(t *testing.T) {
		var gotBase, gotQuote string
		var gotFrom, gotTo time.Time
		at := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
		svc := &mockFXService{listRatesFn: func(base, quote string, from, to time.Time) ([]models.FXRate, error) {
			gotBase, gotQuote, gotFrom, gotTo = base, quote, from, to
			return []models.FXRate{{Base: "USD", Quote: "MYR", Rate: mustFXRate(t, "4.4712"), RecordedAt: at}}, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		rec := doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotBase != "USD" || gotQuote != "MYR" {
			t.Errorf("pair = %s/%s", gotBase, gotQuote)
		}
		if days := gotTo.Sub(gotFrom).Hours() / 24; days < 29.99 || days > 30.01 {
			t.Errorf("default range = %.2f days, want 30", days)
		}
		if d := time.Since(gotTo); d < 0 || d > time.Minute {
			t.Errorf("default to = %s, want about now", gotTo)
		}
		if !strings.Contains(rec.Body.String(), `"rate":"4.4712"`) {
			t.Errorf("rate must be a JSON string: %s", rec.Body.String())
		}
		body := parseJSON(t, rec)
		if body["base"] != "USD" || body["quote"] != "MYR" {
			t.Errorf("unexpected envelope %v", body)
		}
		rates := body["rates"].([]interface{})
		if len(rates) != 1 || rates[0].(map[string]interface{})["recorded_at"] != "2026-11-01T08:00:00Z" {
			t.Errorf("unexpected rates %v", rates)
		}
	})

	t.Run("empty history is an empty array", func(t *testing.T) {
		r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
		rec := doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rates":[]`) {
			t.Fatalf("expected 200 with rates [], got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("accepts RFC 3339 and date-only bounds", func(t *testing.T) {
		var gotFrom, gotTo time.Time
		svc := &mockFXService{listRatesFn: func(_, _ string, from, to time.Time) ([]models.FXRate, error) {
			gotFrom, gotTo = from, to
			return nil, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		rec := doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR&from=2026-10-01&to=2026-10-31", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !gotFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("from = %s", gotFrom)
		}
		if gotTo.Before(time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC)) || !gotTo.Before(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("a date-only to must include that whole day, got %s", gotTo)
		}

		rec = doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR&from=2026-10-01T00:00:00Z&to=2026-10-02T06:00:00Z", "")
		if rec.Code != http.StatusOK || !gotTo.Equal(time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)) {
			t.Errorf("RFC 3339 to not honoured: %d %s", rec.Code, gotTo)
		}
	})

	t.Run("only to given: from is 30 days earlier", func(t *testing.T) {
		var gotFrom, gotTo time.Time
		svc := &mockFXService{listRatesFn: func(_, _ string, from, to time.Time) ([]models.FXRate, error) {
			gotFrom, gotTo = from, to
			return nil, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))
		rec := doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR&to=2026-06-30T00:00:00Z", "")
		if rec.Code != http.StatusOK || gotTo.Sub(gotFrom) != 30*24*time.Hour {
			t.Errorf("got %d from=%s to=%s", rec.Code, gotFrom, gotTo)
		}
	})

	t.Run("range cap", func(t *testing.T) {
		r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
		// Exactly 3700 days is allowed.
		ok := fmt.Sprintf("/fx-rates?base=USD&quote=MYR&from=%s&to=%s",
			"2016-01-01T00:00:00Z", time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 3700).Format(time.RFC3339))
		if rec := doRequest(r, "GET", ok, ""); rec.Code != http.StatusOK {
			t.Errorf("3700 days: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		tooLong := "/fx-rates?base=USD&quote=MYR&from=2016-01-01T00:00:00Z&to=2026-11-03T00:00:00Z"
		rec := doRequest(r, "GET", tooLong, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 beyond 3700 days, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	for name, path := range map[string]string{
		"missing base":     "/fx-rates?quote=MYR",
		"missing quote":    "/fx-rates?base=USD",
		"non-ISO base":     "/fx-rates?base=ZZZ&quote=MYR",
		"lower-case quote": "/fx-rates?base=USD&quote=myr",
		"bad from":         "/fx-rates?base=USD&quote=MYR&from=yesterday",
		"bad to":           "/fx-rates?base=USD&quote=MYR&to=tomorrow",
		"from after to":    "/fx-rates?base=USD&quote=MYR&from=2026-11-02&to=2026-11-01",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
			rec := doRequest(r, "GET", path, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
		})
	}

	t.Run("propagates service errors", func(t *testing.T) {
		svc := &mockFXService{listRatesFn: func(_, _ string, _, _ time.Time) ([]models.FXRate, error) {
			return nil, apperrors.ErrInternalServer
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))
		if rec := doRequest(r, "GET", "/fx-rates?base=USD&quote=MYR", ""); rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestFXHandler_LatestRate(t *testing.T) {
	t.Run("returns the quote", func(t *testing.T) {
		at := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
		var from, to string
		svc := &mockFXService{rateFn: func(f, tq string, _ time.Time) (*services.FXQuote, error) {
			from, to = f, tq
			return &services.FXQuote{Base: f, Quote: tq, Rate: mustFXRate(t, "0.2236"), RecordedAt: at, Stale: true, Path: services.FXPathInverse}, nil
		}}
		r := setupFXRouter(NewFXHandler(svc, &mockAuditService{}))

		rec := doRequest(r, "GET", "/fx-rates/latest?base=MYR&quote=USD", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if from != "MYR" || to != "USD" {
			t.Errorf("pair = %s/%s", from, to)
		}
		if !strings.Contains(rec.Body.String(), `"rate":"0.2236"`) {
			t.Errorf("rate must be a JSON string: %s", rec.Body.String())
		}
		body := parseJSON(t, rec)
		if body["base"] != "MYR" || body["quote"] != "USD" || body["stale"] != true || body["path"] != "inverse" ||
			body["recorded_at"] != "2026-11-03T08:00:00Z" {
			t.Errorf("unexpected body %v", body)
		}
	})

	t.Run("503 when no rate exists", func(t *testing.T) {
		r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
		rec := doRequest(r, "GET", "/fx-rates/latest?base=USD&quote=MYR", "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "FX_RATE_UNAVAILABLE")
	})

	for name, path := range map[string]string{
		"missing quote": "/fx-rates/latest?base=USD",
		"non-ISO base":  "/fx-rates/latest?base=ZZZ&quote=MYR",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			r := setupFXRouter(NewFXHandler(&mockFXService{}, &mockAuditService{}))
			rec := doRequest(r, "GET", path, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
		})
	}
}

func TestParseFXRange(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	from, to, err := parseFXRange("", "", now)
	if err != nil || !to.Equal(now) || !from.Equal(now.AddDate(0, 0, -30)) {
		t.Errorf("defaults: %s %s %v", from, to, err)
	}
}
