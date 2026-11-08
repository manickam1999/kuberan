package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestMultiCurrencyFlow_NativePricesValuedInBase covers plan 024 phase 2: a USD
// security priced in USD is valued in a MYR user's base currency at read time,
// and a price labelled with the wrong currency is rejected.
func TestMultiCurrencyFlow_NativePricesValuedInBase(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "multicurrency@test.com", "password123")

	// A USD security.
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"NVDA","name":"Nvidia","asset_type":"stock","currency":"USD","exchange":"NASDAQ"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create security: %d %s", rec.Code, rec.Body.String())
	}
	securityID := parseJSON(t, rec)["security"].(map[string]interface{})["id"].(string)

	// A MYR investment account holding 10 units.
	rec = app.request("POST", "/api/v1/accounts/investment", `{"name":"Brokerage","broker":"IBKR"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":10,"purchase_price":10000,"fx_rate":"4.4712"}`, accountID, securityID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add investment: %d %s", rec.Code, rec.Body.String())
	}
	investmentID := parseJSON(t, rec)["investment"].(map[string]interface{})["id"].(string)

	// Rate USD->MYR 4.4712 and a USD price of 170.00.
	now := time.Now().UTC().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates",
		fmt.Sprintf(`{"rates":[{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":%q}]}`, now))
	if rec.Code != http.StatusOK {
		t.Fatalf("post rate: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":17000,"currency":"USD","recorded_at":%q}]}`, securityID, now))
	if rec.Code != http.StatusOK {
		t.Fatalf("post USD price: %d %s", rec.Code, rec.Body.String())
	}
	res := parseJSON(t, rec)
	if res["recorded"].(float64) != 1 || len(res["rejected"].([]interface{})) != 0 {
		t.Fatalf("unexpected record result: %v", res)
	}

	// Portfolio is valued in MYR: 10 x 17000 = 170000 USD cents -> 760104 MYR cents.
	rec = app.request("GET", "/api/v1/investments/portfolio", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("portfolio: %d %s", rec.Code, rec.Body.String())
	}
	pf := parseJSON(t, rec)["portfolio"].(map[string]interface{})
	if pf["total_value"].(float64) != 760104 {
		t.Errorf("total_value = %v, want 760104", pf["total_value"])
	}
	if pf["currency"] != "MYR" || len(pf["unconverted"].([]interface{})) != 0 {
		t.Errorf("currency/unconverted = %v %v", pf["currency"], pf["unconverted"])
	}

	// The holding keeps current_price in the base currency and adds the native price.
	rec = app.request("GET", "/api/v1/investments/"+investmentID, "", token)
	inv := parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["current_price"].(float64) != 76010 || inv["current_price_native"].(float64) != 17000 ||
		inv["price_currency"] != "USD" || inv["fx_rate"] != "4.4712" || inv["fx_missing"] != false {
		t.Errorf("investment pricing = %v", inv)
	}

	// A price labelled MYR for the USD security is rejected as a mismatch.
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":99999,"currency":"MYR","recorded_at":%q}]}`,
			securityID, time.Now().UTC().Add(time.Minute).Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("mismatched price must still return 200, got %d: %s", rec.Code, rec.Body.String())
	}
	res = parseJSON(t, rec)
	rejected := res["rejected"].([]interface{})
	if res["recorded"].(float64) != 0 || len(rejected) != 1 {
		t.Fatalf("expected 0 recorded and 1 rejected, got %v", res)
	}
	r0 := rejected[0].(map[string]interface{})
	if r0["reason"] != "currency_mismatch" || r0["currency"] != "MYR" || r0["expected_currency"] != "USD" {
		t.Errorf("rejected entry = %v", r0)
	}

	// Correcting the currency is locked by the stored price unless it is reset.
	rec = app.pipelineRequest("PATCH", "/api/v1/pipeline/securities/"+securityID, `{"currency":"MYR"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 SECURITY_CURRENCY_LOCKED, got %d: %s", rec.Code, rec.Body.String())
	}
	// The investment above is a native-era trade (it has a frozen FX rate), so even
	// reset_prices cannot relabel the security (phase 3a).
	rec = app.pipelineRequest("PATCH", "/api/v1/pipeline/securities/"+securityID, `{"currency":"MYR","reset_prices":true}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for a security with native-era trades, got %d: %s", rec.Code, rec.Body.String())
	}

	// A security with prices but no trades can still be reset.
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"AMD","name":"AMD","asset_type":"stock","currency":"USD","exchange":"NASDAQ"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create second security: %d %s", rec.Code, rec.Body.String())
	}
	otherID := parseJSON(t, rec)["security"].(map[string]interface{})["id"].(string)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":10000,"currency":"USD","recorded_at":%q}]}`, otherID, now))
	if rec.Code != http.StatusOK {
		t.Fatalf("post price: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.pipelineRequest("PATCH", "/api/v1/pipeline/securities/"+otherID, `{"currency":"MYR","reset_prices":true}`)
	if rec.Code != http.StatusOK || parseJSON(t, rec)["prices_deleted"].(float64) != 1 {
		t.Errorf("reset: %d %s", rec.Code, rec.Body.String())
	}
}
