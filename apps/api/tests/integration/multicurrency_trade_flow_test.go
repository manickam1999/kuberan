package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestMultiCurrencyTradeFlow covers plan 024 phase 3a end to end for a MYR user:
// a USD security is bought from a USD cash account, a MYR account cannot pay for
// it without stating the amount, and a partial sale is deposited into the MYR
// account with deposit_amount. The USD account is funded by a MYR to USD
// transfer with to_amount.
func TestMultiCurrencyTradeFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "trades@test.com", "password123")

	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"NVDA","name":"Nvidia","asset_type":"stock","currency":"USD","exchange":"NASDAQ"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create security: %d %s", rec.Code, rec.Body.String())
	}
	securityID := parseJSON(t, rec)["security"].(map[string]interface{})["id"].(string)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates",
		fmt.Sprintf(`{"rates":[{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":%q}]}`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("post rate: %d %s", rec.Code, rec.Body.String())
	}

	account := func(path, body string) string {
		t.Helper()
		rec := app.request("POST", path, body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create account %s: %d %s", body, rec.Code, rec.Body.String())
		}
		return parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	}
	myrCash := account("/api/v1/accounts/cash", `{"name":"Maybank","currency":"MYR","initial_balance":1000000}`)
	usdCash := account("/api/v1/accounts/cash", `{"name":"USD wallet","currency":"USD"}`)
	brokerage := account("/api/v1/accounts/investment", `{"name":"IBKR","broker":"IBKR"}`)

	balance := func(id string) float64 {
		t.Helper()
		rec := app.request("GET", "/api/v1/accounts/"+id, "", token)
		return parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64)
	}

	// A cross-currency transfer without to_amount is refused, with a suggestion
	// from the market rate and nothing moves.
	transferBody := func(extra string) string {
		return fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":894240%s}`, myrCash, usdCash, extra)
	}
	rec = app.request("POST", "/api/v1/transactions/transfer", transferBody(""), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 FX_AMOUNT_REQUIRED, got %d: %s", rec.Code, rec.Body.String())
	}
	te := parseJSON(t, rec)["error"].(map[string]interface{})
	td, _ := te["details"].(map[string]interface{})
	if te["code"] != "FX_AMOUNT_REQUIRED" || td["currency"] != "USD" || td["field"] != "to_amount" || td["suggested_amount"] == nil {
		t.Errorf("transfer error = %v", te)
	}
	if got := balance(myrCash); got != 1000000 {
		t.Errorf("a refused transfer must not move money: MYR balance %.0f", got)
	}

	// Fund the USD account with 2000.00 USD by moving 8942.40 MYR.
	rec = app.request("POST", "/api/v1/transactions/transfer", transferBody(`,"to_amount":200000,"description":"Fund USD"`), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fund USD account: %d %s", rec.Code, rec.Body.String())
	}
	tr := parseJSON(t, rec)["transaction"].(map[string]interface{})
	if tr["to_amount"].(float64) != 200000 || tr["implied_rate"] != "0.2236536053" {
		t.Errorf("transfer = %v", tr)
	}
	if got := balance(usdCash); got != 200000 {
		t.Errorf("USD balance = %.0f, want 200000", got)
	}
	if got := balance(myrCash); got != 105760 {
		t.Errorf("MYR balance = %.0f, want 105760", got)
	}

	// Initial position: 10 @ 100.00 USD at the market rate: 100000 USD cents -> 447120 MYR cents.
	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":10,"purchase_price":10000}`, brokerage, securityID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add investment: %d %s", rec.Code, rec.Body.String())
	}
	inv := parseJSON(t, rec)["investment"].(map[string]interface{})
	investmentID := inv["id"].(string)
	if inv["cost_basis"].(float64) != 100000 || inv["cost_basis_base"].(float64) != 447120 || inv["currency"] != "USD" {
		t.Errorf("initial holding = %v", inv)
	}

	// Buy 5 @ 120.00 USD from the USD account: 60000 USD cents debited, base 268272.
	buyURL := fmt.Sprintf("/api/v1/investments/%s/buy", investmentID)
	date := time.Now().UTC().Format(time.RFC3339)
	rec = app.request("POST", buyURL,
		fmt.Sprintf(`{"date":%q,"quantity":5,"price_per_unit":12000,"funding_account_id":%q}`, date, usdCash), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("buy from USD account: %d %s", rec.Code, rec.Body.String())
	}
	buy := parseJSON(t, rec)["transaction"].(map[string]interface{})
	if buy["fx_rate_to_base"] != "4.4712" || buy["base_amount"].(float64) != 268272 || buy["currency"] != "USD" {
		t.Errorf("buy = %v", buy)
	}
	if got := balance(usdCash); got != 140000 {
		t.Errorf("USD balance = %.0f, want 140000", got)
	}

	// Paying from the MYR account without stating the amount is refused, with a suggestion.
	rec = app.request("POST", buyURL,
		fmt.Sprintf(`{"date":%q,"quantity":1,"price_per_unit":10000,"funding_account_id":%q}`, date, myrCash), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 FX_AMOUNT_REQUIRED, got %d: %s", rec.Code, rec.Body.String())
	}
	e := parseJSON(t, rec)["error"].(map[string]interface{})
	d, _ := e["details"].(map[string]interface{})
	if e["code"] != "FX_AMOUNT_REQUIRED" || d["currency"] != "MYR" || d["field"] != "funding_amount" ||
		d["suggested_amount"].(float64) != 44712 || d["rate"] != "4.4712" {
		t.Errorf("error = %v", e)
	}
	if got := balance(myrCash); got != 105760 {
		t.Errorf("a refused buy must not move money: MYR balance %.0f", got)
	}

	// Sell 6 @ 150.00 USD into the MYR account: proceeds 90000 USD cents, 400000 MYR cents deposited.
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/sell", investmentID),
		fmt.Sprintf(`{"date":%q,"quantity":6,"price_per_unit":15000,"deposit_account_id":%q,"deposit_amount":400000}`, date, myrCash), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("sell into MYR account: %d %s", rec.Code, rec.Body.String())
	}
	sell := parseJSON(t, rec)["transaction"].(map[string]interface{})
	if sell["total_amount"].(float64) != 90000 || sell["base_amount"].(float64) != 400000 || sell["fx_rate_to_base"] != "4.4444444444" {
		t.Errorf("sell = %v", sell)
	}
	// native: 90000 - round(160000*0.4) = 26000; base: 400000 - round(715392*0.4) = 400000 - 286157 = 113843.
	if sell["realized_gain_loss"].(float64) != 26000 || sell["realized_gain_loss_base"].(float64) != 113843 {
		t.Errorf("realized = %v / %v", sell["realized_gain_loss"], sell["realized_gain_loss_base"])
	}
	if got := balance(myrCash); got != 505760 {
		t.Errorf("MYR balance = %.0f, want 505760", got)
	}
	if got := balance(usdCash); got != 140000 {
		t.Errorf("USD balance changed by the MYR deposit: %.0f", got)
	}

	// The holding: native and base figures.
	rec = app.request("GET", "/api/v1/investments/"+investmentID, "", token)
	h := parseJSON(t, rec)["investment"].(map[string]interface{})
	if h["quantity"].(float64) != 9 || h["cost_basis"].(float64) != 96000 || h["cost_basis_base"].(float64) != 429235 ||
		h["realized_gain_loss"].(float64) != 26000 || h["realized_gain_loss_base"].(float64) != 113843 || h["currency"] != "USD" {
		t.Errorf("holding = %v", h)
	}

	// Portfolio price/FX split at 170.00 USD x 9 = 153000 USD cents -> 684094 MYR cents.
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":17000,"currency":"USD","recorded_at":%q}]}`, securityID, time.Now().UTC().Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("post price: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/investments/portfolio", "", token)
	pf := parseJSON(t, rec)["portfolio"].(map[string]interface{})
	value, cost := pf["total_value"].(float64), pf["total_cost_basis"].(float64)
	price, fxr := pf["total_price_return"].(float64), pf["total_fx_return"].(float64)
	if value != 684094 || cost != 429235 {
		t.Errorf("value %.0f cost %.0f, want 684094 / 429235", value, cost)
	}
	if price != 254858 || fxr != 0 {
		t.Errorf("price return %.0f fx return %.0f, want 254858 / 0", price, fxr)
	}
	if diff := price + fxr - (value - cost); diff < -1 || diff > 1 {
		t.Errorf("price + fx != value - cost (diff %.0f)", diff)
	}
	if pf["total_realized_gain_loss"].(float64) != 113843 {
		t.Errorf("realized = %v, want 113843", pf["total_realized_gain_loss"])
	}
}
