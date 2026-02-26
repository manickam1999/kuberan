package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityFlow_FullLifecycle(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "secflow@test.com", "password123")

	// Step 1: Create security via pipeline endpoint (API key auth)
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock","currency":"USD","exchange":"NASDAQ"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating security, got %d: %s", rec.Code, rec.Body.String())
	}
	secResult := parseJSON(t, rec)
	security := secResult["security"].(map[string]interface{})
	securityID := security["id"].(string)

	if security["symbol"] != "AAPL" {
		t.Errorf("expected symbol AAPL, got %v", security["symbol"])
	}
	if security["name"] != "Apple Inc." {
		t.Errorf("expected name Apple Inc., got %v", security["name"])
	}
	if security["asset_type"] != "stock" {
		t.Errorf("expected asset_type stock, got %v", security["asset_type"])
	}
	if security["exchange"] != "NASDAQ" {
		t.Errorf("expected exchange NASDAQ, got %v", security["exchange"])
	}

	// Step 2: List securities (JWT auth) — verify 1 result
	rec = app.request("GET", "/api/v1/securities", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing securities, got %d: %s", rec.Code, rec.Body.String())
	}
	listResult := parseJSON(t, rec)
	if listResult["total_items"].(float64) != 1 {
		t.Errorf("expected 1 security, got %.0f", listResult["total_items"].(float64))
	}
	data := listResult["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 item in data, got %d", len(data))
	}
	listedSec := data[0].(map[string]interface{})
	if listedSec["symbol"] != "AAPL" {
		t.Errorf("expected listed symbol AAPL, got %v", listedSec["symbol"])
	}

	// Step 3: Get security by ID — verify fields
	rec = app.request("GET", fmt.Sprintf("/api/v1/securities/%s", securityID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 getting security, got %d: %s", rec.Code, rec.Body.String())
	}
	getSec := parseJSON(t, rec)["security"].(map[string]interface{})
	if getSec["symbol"] != "AAPL" {
		t.Errorf("expected symbol AAPL, got %v", getSec["symbol"])
	}
	if getSec["currency"] != "USD" {
		t.Errorf("expected currency USD, got %v", getSec["currency"])
	}

	// Step 4: Record 3 prices via pipeline endpoint (API key auth)
	now := time.Now().UTC()
	t1 := now.Add(-2 * time.Hour).Format(time.RFC3339)
	t2 := now.Add(-1 * time.Hour).Format(time.RFC3339)
	t3 := now.Format(time.RFC3339)

	pricesBody := fmt.Sprintf(`{"prices":[
		{"security_id":%q,"price":17500,"recorded_at":%q},
		{"security_id":%q,"price":17600,"recorded_at":%q},
		{"security_id":%q,"price":17700,"recorded_at":%q}
	]}`, securityID, t1, securityID, t2, securityID, t3)

	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices", pricesBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 recording prices, got %d: %s", rec.Code, rec.Body.String())
	}
	priceResult := parseJSON(t, rec)
	if priceResult["prices_recorded"].(float64) != 3 {
		t.Errorf("expected 3 prices recorded, got %.0f", priceResult["prices_recorded"].(float64))
	}

	// Step 5: Get price history (JWT auth) — verify 3 entries ordered by recorded_at DESC
	fromDate := now.Add(-3 * time.Hour).Format(time.RFC3339)
	toDate := now.Add(1 * time.Hour).Format(time.RFC3339)

	rec = app.request("GET",
		fmt.Sprintf("/api/v1/securities/%s/prices?from_date=%s&to_date=%s", securityID, fromDate, toDate),
		"", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 getting price history, got %d: %s", rec.Code, rec.Body.String())
	}
	historyResult := parseJSON(t, rec)
	if historyResult["total_items"].(float64) != 3 {
		t.Errorf("expected 3 price entries, got %.0f", historyResult["total_items"].(float64))
	}
	prices := historyResult["data"].([]interface{})
	if len(prices) != 3 {
		t.Fatalf("expected 3 prices in data, got %d", len(prices))
	}

	// Verify ordered by recorded_at DESC (most recent first)
	firstPrice := prices[0].(map[string]interface{})
	lastPrice := prices[2].(map[string]interface{})
	if firstPrice["price"].(float64) != 17700 {
		t.Errorf("expected first price 17700 (most recent), got %.0f", firstPrice["price"].(float64))
	}
	if lastPrice["price"].(float64) != 17500 {
		t.Errorf("expected last price 17500 (oldest), got %.0f", lastPrice["price"].(float64))
	}
}

func TestSecurityFlow_ProviderSymbolRoundTrip(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "provsym@test.com", "password123")

	// Create security with provider_symbol
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"CIMB","name":"CIMB Group","asset_type":"stock","currency":"MYR","exchange":"BURSA","provider_symbol":"1023.KL"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	secResult := parseJSON(t, rec)
	security := secResult["security"].(map[string]interface{})
	securityID := security["id"].(string)

	if security["provider_symbol"] != "1023.KL" {
		t.Errorf("expected provider_symbol 1023.KL in create response, got %v", security["provider_symbol"])
	}

	// Get security by ID — verify provider_symbol round-trips
	rec = app.request("GET", fmt.Sprintf("/api/v1/securities/%s", securityID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	getSec := parseJSON(t, rec)["security"].(map[string]interface{})
	if getSec["provider_symbol"] != "1023.KL" {
		t.Errorf("expected provider_symbol 1023.KL on GET, got %v", getSec["provider_symbol"])
	}

	// List all via pipeline — verify provider_symbol is returned
	rec = app.pipelineRequest("GET", "/api/v1/pipeline/securities", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	listResult := parseJSON(t, rec)
	securities := listResult["securities"].([]interface{})
	found := false
	for _, s := range securities {
		sec := s.(map[string]interface{})
		if sec["symbol"] == "CIMB" {
			if sec["provider_symbol"] != "1023.KL" {
				t.Errorf("expected provider_symbol 1023.KL in pipeline list, got %v", sec["provider_symbol"])
			}
			found = true
		}
	}
	if !found {
		t.Error("CIMB not found in pipeline securities list")
	}
}

func TestSecurityFlow_DuplicateSymbolExchange(t *testing.T) {
	app := setupApp(t)

	// Step 1: Create security (AAPL, NYSE)
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock","exchange":"NYSE"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 2: Create same security again — expect 409
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock","exchange":"NYSE"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d: %s", rec.Code, rec.Body.String())
	}
	errObj := parseJSON(t, rec)["error"].(map[string]interface{})
	if errObj["code"] != "DUPLICATE_SECURITY" {
		t.Errorf("expected DUPLICATE_SECURITY, got %v", errObj["code"])
	}

	// Step 3: Create AAPL on NASDAQ — expect 201 (different exchange, allowed)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities",
		`{"symbol":"AAPL","name":"Apple Inc.","asset_type":"stock","exchange":"NASDAQ"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for different exchange, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityFlow_PipelineAuthRequired(t *testing.T) {
	app := setupApp(t)

	// Pipeline endpoint without API key should return 401
	body := `{"symbol":"TEST","name":"Test","asset_type":"stock"}`
	rec := app.request("POST", "/api/v1/pipeline/securities", body, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without API key, got %d: %s", rec.Code, rec.Body.String())
	}

	// Pipeline endpoint with wrong API key should return 401
	req := httptest.NewRequest("POST", "/api/v1/pipeline/securities", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong API key, got %d: %s", w.Code, w.Body.String())
	}
}
