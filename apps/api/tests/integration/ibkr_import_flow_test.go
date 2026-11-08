package integration

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	ibkrFixtureDir = "../../internal/importers/ibkr/testdata/"
)

// importRequest posts the two fixture statements to the IBKR import endpoint.
func (app *testApp) importRequest(t *testing.T, token string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, name := range []string{"statement_2026.csv", "statement_2025.csv"} {
		content, err := os.ReadFile(ibkrFixtureDir + name)
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(content)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/imports/ibkr", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

// TestIBKRImportFlow runs the IBKR import end to end over HTTP: a dry run, an
// apply that rebuilds the holdings and the USD cash ledger and re-points the
// legacy MYR transfer, an idempotent second apply, and a refused apply.
func TestIBKRImportFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "ibkr@test.com", "password123")

	for sym, ex := range map[string]string{
		"CCJ": "NYSE", "CHA": "NASDAQ", "IBKR": "NASDAQ", "NVDA": "NASDAQ", "NVO": "NYSE", "SPCX": "NASDAQ",
		"VUAA": "LSE", "VWRA": "LSE", "RBOT": "LSE", "SOUN": "NASDAQ",
	} {
		rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
			fmt.Sprintf(`{"symbol":%q,"name":%q,"asset_type":"stock","currency":"USD","exchange":%q}`, sym, sym+" Inc", ex))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create security %s: %d %s", sym, rec.Code, rec.Body.String())
		}
	}
	var rates []string
	for d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC); !d.After(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		rates = append(rates, fmt.Sprintf(`{"base":"USD","quote":"MYR","rate":"4.5","recorded_at":%q}`, d.Format(time.RFC3339)))
	}
	if rec := app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates", `{"rates":[`+strings.Join(rates, ",")+`]}`); rec.Code != http.StatusOK {
		t.Fatalf("post rates: %d %s", rec.Code, rec.Body.String())
	}

	account := func(path, body string) string {
		t.Helper()
		rec := app.request("POST", path, body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create account %s: %d %s", body, rec.Code, rec.Body.String())
		}
		return parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	}
	bank := account("/api/v1/accounts/cash", `{"name":"Maybank","currency":"MYR","initial_balance":10000000}`)
	legacy := account("/api/v1/accounts/cash", `{"name":"IBKR Cash","currency":"MYR"}`)
	usd := account("/api/v1/accounts/cash", `{"name":"IBKR Cash (USD)","currency":"USD"}`)
	brokerage := account("/api/v1/accounts/investment", `{"name":"IBKR","broker":"IBKR"}`)

	rec := app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":3300000,"description":"top up","date":"2026-06-03T00:00:00Z"}`, bank, legacy), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("transfer: %d %s", rec.Code, rec.Body.String())
	}

	balance := func(id string) float64 {
		t.Helper()
		return parseJSON(t, app.request("GET", "/api/v1/accounts/"+id, "", token))["account"].(map[string]interface{})["balance"].(float64)
	}
	fields := func(apply string) map[string]string {
		return map[string]string{
			"investment_account_id": brokerage, "cash_account_id": usd, "legacy_cash_account_id": legacy,
			"cash_start_date": "2026-01-01", "apply": apply,
		}
	}

	if rec := app.importRequest(t, "", fields("false")); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated import: %d", rec.Code)
	}

	// Dry run: a report, no writes.
	rec = app.importRequest(t, token, fields("false"))
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	report := parseJSON(t, rec)
	if report["applied"] != false || len(report["errors"].([]interface{})) != 0 {
		t.Fatalf("dry run report: %v", report)
	}
	if balance(usd) != 0 || balance(legacy) != 3300000 {
		t.Errorf("dry run moved money: usd %v legacy %v", balance(usd), balance(legacy))
	}

	// Apply.
	rec = app.importRequest(t, token, fields("true"))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}
	report = parseJSON(t, rec)
	if report["applied"] != true {
		t.Fatalf("apply report: %v", report)
	}
	cash := report["cash"].(map[string]interface{})
	matched := cash["deposits_matched"].([]interface{})
	if len(matched) != 1 || matched[0].(map[string]interface{})["amount"].(float64) != 746152 {
		t.Errorf("deposits_matched: %v", matched)
	}
	cps := cash["checkpoints"].([]interface{})
	last := cps[len(cps)-1].(map[string]interface{})
	if balance(usd) != last["ledger"].(float64) || last["ibkr"].(float64) != 12055 {
		t.Errorf("usd balance %v, checkpoint %v", balance(usd), last)
	}
	if balance(legacy) != 0 {
		t.Errorf("legacy balance %v, want 0", balance(legacy))
	}
	usdBefore := balance(usd)

	// Second apply: no change.
	if rec := app.importRequest(t, token, fields("true")); rec.Code != http.StatusOK {
		t.Fatalf("second apply: %d %s", rec.Code, rec.Body.String())
	}
	if balance(usd) != usdBefore || balance(legacy) != 0 {
		t.Errorf("second apply changed balances: usd %v legacy %v", balance(usd), balance(legacy))
	}

	// A refused apply: the cash account is in the wrong currency for the statements.
	f := fields("true")
	f["cash_account_id"] = bank
	if rec := app.importRequest(t, token, f); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong-currency cash account: %d %s", rec.Code, rec.Body.String())
	}
	// Another user's account is not found.
	other, _, _ := app.registerUser(t, "ibkr-other@test.com", "password123")
	if rec := app.importRequest(t, other, fields("false")); rec.Code != http.StatusNotFound {
		t.Errorf("foreign accounts: %d %s", rec.Code, rec.Body.String())
	}
}
