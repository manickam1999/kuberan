package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestFXFlow_PipelinePostThenInverseLookup(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fxflow@test.com", "password123")

	// Nothing recorded yet: 503.
	rec := app.request("GET", "/api/v1/fx-rates/latest?base=MYR&quote=USD", "", token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before any rate, got %d: %s", rec.Code, rec.Body.String())
	}

	// The pipeline publishes USD->MYR.
	recordedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Format(time.RFC3339)
	body := fmt.Sprintf(`{"rates":[{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":%q,"source":"yahoo"}]}`, recordedAt)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 recording rates, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["recorded"]; got != float64(1) {
		t.Errorf("expected recorded=1, got %v", got)
	}

	// MYR->USD resolves as the inverse of the stored row.
	rec = app.request("GET", "/api/v1/fx-rates/latest?base=MYR&quote=USD", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	latest := parseJSON(t, rec)
	if latest["path"] != "inverse" || latest["rate"] != "0.2236536053" || latest["stale"] != false {
		t.Errorf("unexpected inverse quote: %v", latest)
	}
	if latest["recorded_at"] != recordedAt {
		t.Errorf("recorded_at = %v, want %s", latest["recorded_at"], recordedAt)
	}

	// The direct pair resolves directly, and the history lists the stored row.
	rec = app.request("GET", "/api/v1/fx-rates/latest?base=USD&quote=MYR", "", token)
	if direct := parseJSON(t, rec); rec.Code != http.StatusOK || direct["path"] != "direct" || direct["rate"] != "4.4712" {
		t.Errorf("unexpected direct quote: %d %v", rec.Code, direct)
	}
	rec = app.request("GET", "/api/v1/fx-rates?base=USD&quote=MYR", "", token)
	history := parseJSON(t, rec)
	if rec.Code != http.StatusOK || len(history["rates"].([]interface{})) != 1 {
		t.Fatalf("unexpected history: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/fx-rates?base=MYR&quote=USD", "", token)
	if inv := parseJSON(t, rec); len(inv["rates"].([]interface{})) != 0 {
		t.Errorf("history is direct-pair only, got %v", inv["rates"])
	}

	// Re-posting the same key updates the rate in place.
	body = fmt.Sprintf(`{"rates":[{"base":"USD","quote":"MYR","rate":"4.5","recorded_at":%q}]}`, recordedAt)
	if rec = app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates", body); rec.Code != http.StatusOK {
		t.Fatalf("upsert: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/fx-rates/latest?base=USD&quote=MYR", "", token)
	if got := parseJSON(t, rec)["rate"]; got != "4.5" {
		t.Errorf("rate after upsert = %v, want 4.5", got)
	}
}

func TestFXFlow_AuthAndPairs(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fxpairs@test.com", "password123")

	if rec := app.request("GET", "/api/v1/fx-rates/latest?base=USD&quote=MYR", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("fx-rates requires a JWT, got %d", rec.Code)
	}
	if rec := app.request("POST", "/api/v1/pipeline/fx-rates", `{"rates":[]}`, token); rec.Code == http.StatusOK {
		t.Errorf("pipeline endpoints must not accept a user JWT, got %d", rec.Code)
	}

	// A MYR user with a USD cash account needs USD->MYR and MYR->USD.
	rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Wallet","currency":"USD","initial_balance":1000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating account: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.pipelineRequest("GET", "/api/v1/pipeline/fx-pairs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("fx-pairs: %d %s", rec.Code, rec.Body.String())
	}
	out := parseJSON(t, rec)
	pairs := out["pairs"].([]interface{})
	got := map[string]bool{}
	for _, p := range pairs {
		m := p.(map[string]interface{})
		got[m["base"].(string)+"/"+m["quote"].(string)] = true
	}
	if !got["USD/MYR"] || !got["MYR/USD"] || len(got) != 2 {
		t.Errorf("unexpected pairs %v", got)
	}
	if bases := out["base_currencies"].([]interface{}); len(bases) != 1 || bases[0] != "MYR" {
		t.Errorf("unexpected base currencies %v", bases)
	}
}

func TestFXFlow_BaseCurrencyProfile(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fxbase@test.com", "password123")

	rec := app.request("GET", "/api/v1/profile", "", token)
	user := parseJSON(t, rec)["user"].(map[string]interface{})
	if user["base_currency"] != "MYR" {
		t.Fatalf("new users default to MYR, got %v", user["base_currency"])
	}

	// SGD has no rate to USD yet.
	rec = app.request("PATCH", "/api/v1/profile", `{"base_currency":"SGD"}`, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if code := parseJSON(t, rec)["error"].(map[string]interface{})["code"]; code != "UNSUPPORTED_CURRENCY" {
		t.Errorf("expected UNSUPPORTED_CURRENCY, got %v", code)
	}

	at := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	body := fmt.Sprintf(`{"rates":[{"base":"USD","quote":"SGD","rate":"1.3","recorded_at":%q},{"base":"USD","quote":"JPY","rate":"150","recorded_at":%q}]}`, at, at)
	if rec = app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates", body); rec.Code != http.StatusOK {
		t.Fatalf("recording rates: %d %s", rec.Code, rec.Body.String())
	}

	rec = app.request("PATCH", "/api/v1/profile", `{"base_currency":"SGD"}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 once a rate exists, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["user"].(map[string]interface{})["base_currency"]; got != "SGD" {
		t.Errorf("base_currency = %v, want SGD", got)
	}

	// JPY has a rate but is not a 2-decimal currency.
	if rec = app.request("PATCH", "/api/v1/profile", `{"base_currency":"JPY"}`, token); rec.Code != http.StatusBadRequest {
		t.Errorf("JPY must be rejected, got %d", rec.Code)
	}
	// USD is always supported.
	if rec = app.request("PATCH", "/api/v1/profile", `{"base_currency":"USD"}`, token); rec.Code != http.StatusOK {
		t.Errorf("USD must be accepted, got %d: %s", rec.Code, rec.Body.String())
	}
	// Auth responses embed the base currency too.
	rec = app.request("GET", "/api/v1/profile", "", token)
	if got := parseJSON(t, rec)["user"].(map[string]interface{})["base_currency"]; got != "USD" {
		t.Errorf("profile base_currency = %v, want USD", got)
	}
}
