package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestPortfolioSnapshotFlow_ComputeAndQuery(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "snapshot@test.com", "password123")

	// Step 1: Create cash account with $5000 balance (500000 cents)
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Checking","initial_balance":500000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating cash account, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 2: Create investment account
	rec = app.request("POST", "/api/v1/accounts/investment",
		`{"name":"Brokerage"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating investment account, got %d: %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Step 3: Create security and add investment (10 shares @ $150 = $1500)
	securityID := app.createSecurity(t, "AAPL", "Apple Inc.", "stock")
	rec = app.request("POST", "/api/v1/investments",
		fmt.Sprintf(`{"account_id":%q,"security_id":%q,"quantity":10,"purchase_price":15000}`,
			accountID, securityID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 adding investment, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 4: Record live price via pipeline ($150/share)
	priceTime := time.Now().UTC().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/securities/prices",
		fmt.Sprintf(`{"prices":[{"security_id":%q,"price":15000,"recorded_at":%q}]}`, securityID, priceTime))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for pipeline price, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 6: Compute snapshots via pipeline endpoint
	recordedAt := time.Now().UTC().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots",
		fmt.Sprintf(`{"recorded_at":%q}`, recordedAt))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 computing snapshots, got %d: %s", rec.Code, rec.Body.String())
	}
	computeResult := parseJSON(t, rec)
	if computeResult["snapshots_recorded"].(float64) != 1 {
		t.Errorf("expected 1 snapshot, got %.0f", computeResult["snapshots_recorded"].(float64))
	}

	// Step 7: Get snapshots (JWT auth) — verify correct breakdown
	fromDate := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	toDate := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	rec = app.request("GET",
		fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate),
		"", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 getting snapshots, got %d: %s", rec.Code, rec.Body.String())
	}
	snapResult := parseJSON(t, rec)
	if snapResult["total_items"].(float64) != 1 {
		t.Errorf("expected 1 snapshot, got %.0f", snapResult["total_items"].(float64))
	}
	data := snapResult["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 item in data, got %d", len(data))
	}

	snapshot := data[0].(map[string]interface{})

	// Cash balance: $5000 = 500000 cents
	if snapshot["cash_balance"].(float64) != 500000 {
		t.Errorf("expected cash_balance 500000, got %.0f", snapshot["cash_balance"].(float64))
	}

	// Investment value: 10 shares * $150 (15000 cents) = 150000 cents
	if snapshot["investment_value"].(float64) != 150000 {
		t.Errorf("expected investment_value 150000, got %.0f", snapshot["investment_value"].(float64))
	}

	// No debt
	if snapshot["debt_balance"].(float64) != 0 {
		t.Errorf("expected debt_balance 0, got %.0f", snapshot["debt_balance"].(float64))
	}

	// Total net worth: 500000 + 150000 - 0 = 650000
	if snapshot["total_net_worth"].(float64) != 650000 {
		t.Errorf("expected total_net_worth 650000, got %.0f", snapshot["total_net_worth"].(float64))
	}
}

func TestPortfolioSnapshotFlow_MultipleUsersComputed(t *testing.T) {
	app := setupApp(t)

	// Register two users with different balances
	token1, _, _ := app.registerUser(t, "user1@test.com", "password123")
	token2, _, _ := app.registerUser(t, "user2@test.com", "password123")

	// User 1: $3000 cash
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"User1 Cash","initial_balance":300000}`, token1)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// User 2: $7000 cash
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"User2 Cash","initial_balance":700000}`, token2)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Compute snapshots — should create 2 (one per user)
	recordedAt := time.Now().UTC().Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots",
		fmt.Sprintf(`{"recorded_at":%q}`, recordedAt))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if parseJSON(t, rec)["snapshots_recorded"].(float64) != 2 {
		t.Errorf("expected 2 snapshots, got %.0f", parseJSON(t, rec)["snapshots_recorded"].(float64))
	}

	// Query snapshots for each user — verify isolation
	fromDate := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	toDate := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	// User 1 snapshot: cash_balance = 300000
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate),
		"", token1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	snap1 := parseJSON(t, rec)
	if snap1["total_items"].(float64) != 1 {
		t.Errorf("expected 1 snapshot for user1, got %.0f", snap1["total_items"].(float64))
	}
	s1 := snap1["data"].([]interface{})[0].(map[string]interface{})
	if s1["cash_balance"].(float64) != 300000 {
		t.Errorf("expected user1 cash_balance 300000, got %.0f", s1["cash_balance"].(float64))
	}
	if s1["total_net_worth"].(float64) != 300000 {
		t.Errorf("expected user1 total_net_worth 300000, got %.0f", s1["total_net_worth"].(float64))
	}

	// User 2 snapshot: cash_balance = 700000
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate),
		"", token2)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	snap2 := parseJSON(t, rec)
	if snap2["total_items"].(float64) != 1 {
		t.Errorf("expected 1 snapshot for user2, got %.0f", snap2["total_items"].(float64))
	}
	s2 := snap2["data"].([]interface{})[0].(map[string]interface{})
	if s2["cash_balance"].(float64) != 700000 {
		t.Errorf("expected user2 cash_balance 700000, got %.0f", s2["cash_balance"].(float64))
	}
	if s2["total_net_worth"].(float64) != 700000 {
		t.Errorf("expected user2 total_net_worth 700000, got %.0f", s2["total_net_worth"].(float64))
	}
}
