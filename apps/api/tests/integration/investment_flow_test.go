package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestInvestmentFlow_FullLifecycle(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "invest@test.com", "password123")

	// Step 0: Create security via pipeline
	securityID := app.createSecurity(t, "AAPL", "Apple Inc.", "stock")

	// Step 1: Create investment account
	rec := app.request("POST", "/api/v1/accounts/investment",
		`{"name":"Brokerage","broker":"Fidelity","account_number":"12345"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating investment account, got %d: %s", rec.Code, rec.Body.String())
	}
	acctResult := parseJSON(t, rec)
	account := acctResult["account"].(map[string]interface{})
	accountID := account["id"].(string)

	// Step 2: Add investment holding (10 shares of AAPL at $150/share = $1500 cost basis)
	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":10,"purchase_price":15000}`,
			accountID, securityID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 adding investment, got %d: %s", rec.Code, rec.Body.String())
	}
	invResult := parseJSON(t, rec)
	investment := invResult["investment"].(map[string]interface{})
	investmentID := investment["id"].(string)

	if investment["quantity"].(float64) != 10 {
		t.Errorf("expected quantity 10, got %v", investment["quantity"])
	}
	// Cost basis = 10 * 15000 = 150000 cents
	if investment["cost_basis"].(float64) != 150000 {
		t.Errorf("expected cost basis 150000, got %.0f", investment["cost_basis"].(float64))
	}

	// Step 3: Record additional buy (5 shares at $160/share, $10 fee)
	buyDate := time.Now().Format(time.RFC3339)
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/buy", investmentID),
		fmt.Sprintf(`{"date":%q,"quantity":5,"price_per_unit":16000,"fee":1000,"notes":"Additional buy"}`, buyDate), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for buy, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 4: Verify investment after buy (15 shares, cost basis = 150000 + 5*16000 + 1000 = 231000)
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	inv := parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 15 {
		t.Errorf("expected 15 shares after buy, got %v", inv["quantity"])
	}
	if inv["cost_basis"].(float64) != 231000 {
		t.Errorf("expected cost basis 231000, got %.0f", inv["cost_basis"].(float64))
	}

	// Step 5: Record live price via pipeline ($170/share)
	now := time.Now().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":17000,"recorded_at":%q}]}`, securityID, now))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for pipeline price, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 6: Record sell (5 shares at $170/share, $10 fee)
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/sell", investmentID),
		fmt.Sprintf(`{"date":%q,"quantity":5,"price_per_unit":17000,"fee":1000}`, buyDate), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for sell, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 7: Verify investment after sell (10 shares, cost basis reduced proportionally)
	// Cost basis reduction = 231000 * (5/15) = 77000; remaining = 231000 - 77000 = 154000
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	inv = parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 10 {
		t.Errorf("expected 10 shares after sell, got %v", inv["quantity"])
	}
	if inv["cost_basis"].(float64) != 154000 {
		t.Errorf("expected cost basis 154000 after proportional reduction, got %.0f", inv["cost_basis"].(float64))
	}

	// Step 8: Check portfolio summary
	rec = app.request("GET", "/api/v1/investments/portfolio", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for portfolio, got %d: %s", rec.Code, rec.Body.String())
	}
	portfolio := parseJSON(t, rec)["portfolio"].(map[string]interface{})
	// Total value = 10 shares * 17000 = 170000
	if portfolio["total_value"].(float64) != 170000 {
		t.Errorf("expected portfolio total value 170000, got %.0f", portfolio["total_value"].(float64))
	}
	if portfolio["total_cost_basis"].(float64) != 154000 {
		t.Errorf("expected portfolio cost basis 154000, got %.0f", portfolio["total_cost_basis"].(float64))
	}
	// Gain = 170000 - 154000 = 16000
	if portfolio["total_gain_loss"].(float64) != 16000 {
		t.Errorf("expected gain 16000, got %.0f", portfolio["total_gain_loss"].(float64))
	}

	// Step 9: Verify investment transactions list
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s/transactions", investmentID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	txResult := parseJSON(t, rec)
	// 3 transactions: initial buy + additional buy + sell
	if txResult["total_items"].(float64) != 3 {
		t.Errorf("expected 3 investment transactions, got %.0f", txResult["total_items"].(float64))
	}
}

func TestInvestmentFlow_DividendAndSplit(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "divsplit@test.com", "password123")

	// Create security
	securityID := app.createSecurity(t, "MSFT", "Microsoft Corp", "stock")

	// Create investment account and holding
	rec := app.request("POST", "/api/v1/accounts/investment",
		`{"name":"Dividend Account"}`, token)
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":20,"purchase_price":30000}`,
			accountID, securityID), token)
	investmentID := parseJSON(t, rec)["investment"].(map[string]interface{})["id"].(string)

	// Verify initial state: 20 shares, cost basis = 20 * 30000 = 600000
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	inv := parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 20 {
		t.Errorf("expected 20 shares, got %v", inv["quantity"])
	}
	if inv["cost_basis"].(float64) != 600000 {
		t.Errorf("expected cost basis 600000, got %.0f", inv["cost_basis"].(float64))
	}

	// Record dividend ($2 per share = $40 total)
	now := time.Now().Format(time.RFC3339)
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/dividend", investmentID),
		fmt.Sprintf(`{"date":%q,"amount":4000,"dividend_type":"Cash","notes":"Quarterly dividend"}`, now), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for dividend, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify quantity and cost basis unchanged after dividend
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	inv = parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 20 {
		t.Errorf("expected 20 shares after dividend, got %v", inv["quantity"])
	}
	if inv["cost_basis"].(float64) != 600000 {
		t.Errorf("expected cost basis 600000 after dividend, got %.0f", inv["cost_basis"].(float64))
	}

	// Record 2:1 stock split
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/split", investmentID),
		fmt.Sprintf(`{"date":%q,"split_ratio":2,"notes":"2-for-1 split"}`, now), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for split, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify: quantity doubled (40), cost basis unchanged (600000)
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	inv = parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 40 {
		t.Errorf("expected 40 shares after 2:1 split, got %v", inv["quantity"])
	}
	if inv["cost_basis"].(float64) != 600000 {
		t.Errorf("expected cost basis 600000 unchanged after split, got %.0f", inv["cost_basis"].(float64))
	}

	// Verify investment transactions: initial buy + dividend + split = 3
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s/transactions", investmentID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if parseJSON(t, rec)["total_items"].(float64) != 3 {
		t.Errorf("expected 3 transactions (buy+dividend+split), got %.0f", parseJSON(t, rec)["total_items"].(float64))
	}
}

func TestInvestmentFlow_SellInsufficientShares(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "insuffshares@test.com", "password123")

	securityID := app.createSecurity(t, "GOOG", "Alphabet", "stock")

	rec := app.request("POST", "/api/v1/accounts/investment",
		`{"name":"Small Account"}`, token)
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":5,"purchase_price":10000}`,
			accountID, securityID), token)
	investmentID := parseJSON(t, rec)["investment"].(map[string]interface{})["id"].(string)

	// Try to sell more shares than held
	now := time.Now().Format(time.RFC3339)
	rec = app.request("POST", fmt.Sprintf("/api/v1/investments/%s/sell", investmentID),
		fmt.Sprintf(`{"date":%q,"quantity":10,"price_per_unit":12000}`, now), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for insufficient shares, got %d: %s", rec.Code, rec.Body.String())
	}
	errObj := parseJSON(t, rec)["error"].(map[string]interface{})
	if errObj["code"] != "INSUFFICIENT_SHARES" {
		t.Errorf("expected INSUFFICIENT_SHARES, got %v", errObj["code"])
	}

	// Verify quantity unchanged
	rec = app.request("GET", fmt.Sprintf("/api/v1/investments/%s", investmentID), "", token)
	inv := parseJSON(t, rec)["investment"].(map[string]interface{})
	if inv["quantity"].(float64) != 5 {
		t.Errorf("expected 5 shares unchanged, got %v", inv["quantity"])
	}
}

func TestInvestmentFlow_PortfolioMultipleHoldings(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "multihold@test.com", "password123")

	// Create securities
	aaplID := app.createSecurity(t, "AAPL", "Apple", "stock")
	vooID := app.createSecurity(t, "VOO", "Vanguard S&P 500", "etf")

	// Create investment account
	rec := app.request("POST", "/api/v1/accounts/investment",
		`{"name":"Diversified"}`, token)
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Add stock: 10 shares at $100
	app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":10,"purchase_price":10000}`,
			accountID, aaplID), token)

	// Add ETF: 20 shares at $50
	app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":20,"purchase_price":5000}`,
			accountID, vooID), token)

	// Record live prices via pipeline
	now := time.Now().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":12000,"recorded_at":%q},{"security_id":%q,"price":5500,"recorded_at":%q}]}`,
			aaplID, now, vooID, now))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for pipeline prices, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check portfolio
	rec = app.request("GET", "/api/v1/investments/portfolio", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	portfolio := parseJSON(t, rec)["portfolio"].(map[string]interface{})

	// Stock value: 10 * 12000 = 120000, ETF value: 20 * 5500 = 110000, Total: 230000
	if portfolio["total_value"].(float64) != 230000 {
		t.Errorf("expected total value 230000, got %.0f", portfolio["total_value"].(float64))
	}

	// Stock cost: 10 * 10000 = 100000, ETF cost: 20 * 5000 = 100000, Total: 200000
	if portfolio["total_cost_basis"].(float64) != 200000 {
		t.Errorf("expected total cost basis 200000, got %.0f", portfolio["total_cost_basis"].(float64))
	}

	// Total gain: 230000 - 200000 = 30000
	if portfolio["total_gain_loss"].(float64) != 30000 {
		t.Errorf("expected total gain 30000, got %.0f", portfolio["total_gain_loss"].(float64))
	}

	// Check holdings by type
	holdingsByType := portfolio["holdings_by_type"].(map[string]interface{})
	stockSummary := holdingsByType["stock"].(map[string]interface{})
	if stockSummary["count"].(float64) != 1 {
		t.Errorf("expected 1 stock holding, got %.0f", stockSummary["count"].(float64))
	}
	if stockSummary["value"].(float64) != 120000 {
		t.Errorf("expected stock value 120000, got %.0f", stockSummary["value"].(float64))
	}

	etfSummary := holdingsByType["etf"].(map[string]interface{})
	if etfSummary["count"].(float64) != 1 {
		t.Errorf("expected 1 ETF holding, got %.0f", etfSummary["count"].(float64))
	}
	if etfSummary["value"].(float64) != 110000 {
		t.Errorf("expected ETF value 110000, got %.0f", etfSummary["value"].(float64))
	}
}
