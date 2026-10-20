package integration

// Integration tests for the fixed deposit account flow (plan 020, B10).
//
// Tests open a real FD via the HTTP API, verify the source account is debited,
// then close the FD and verify principal + interest land in the destination account
// and the net worth snapshot is correct.

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// fdFlowDate returns a YYYY-MM-DD string for the given date offset from today.
func fdFlowDate(daysOffset int) string {
	d := time.Now().UTC().AddDate(0, 0, daysOffset)
	return d.Format("2006-01-02")
}

func TestFixedDepositFlow_OpenAndCloseCumulative(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fdcumulative@test.com", "password123")

	// Step 1: Create source cash account with $5000.00
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Bank Account","currency":"USD","initial_balance":500000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bank account: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	bankResult := parseJSON(t, rec)
	bankAcct := bankResult["account"].(map[string]interface{})
	bankID := bankAcct["id"].(string)
	if bankAcct["balance"].(float64) != 500000 {
		t.Errorf("expected bank balance 500000, got %v", bankAcct["balance"])
	}

	// Step 2: Create destination cash account
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Savings Account","currency":"USD"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create destination account: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	destResult := parseJSON(t, rec)
	destAcct := destResult["account"].(map[string]interface{})
	destID := destAcct["id"].(string)

	// Step 3: Open a cumulative FD for $1000.00 from the bank account
	startDate := fdFlowDate(0)
	maturityDate := fdFlowDate(365) // 1 year from today
	fdBody := fmt.Sprintf(`{
		"name": "1-Year Fixed Deposit",
		"currency": "USD",
		"source_account_id": %q,
		"principal": 100000,
		"interest_rate": 5.0,
		"start_date": %q,
		"maturity_date": %q,
		"payout_type": "cumulative",
		"compounding": "annual",
		"linked_account_id": %q
	}`, bankID, startDate, maturityDate, destID)

	rec = app.request("POST", "/api/v1/accounts/fixed-deposit", fdBody, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open FD: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	fdResult := parseJSON(t, rec)
	fdAcct := fdResult["account"].(map[string]interface{})
	fdID := fdAcct["id"].(string)

	// FD balance should equal principal
	if fdAcct["balance"].(float64) != 100000 {
		t.Errorf("expected FD balance 100000, got %v", fdAcct["balance"])
	}
	if fdAcct["type"] != "fixed_deposit" {
		t.Errorf("expected type fixed_deposit, got %v", fdAcct["type"])
	}

	// Step 4: Verify bank account was debited
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", bankID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get bank: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	bankCheck := parseJSON(t, rec)["account"].(map[string]interface{})
	if bankCheck["balance"].(float64) != 400000 {
		t.Errorf("expected bank balance 400000 after FD funding, got %v", bankCheck["balance"])
	}

	// Step 5: Close the FD with an interest override (simulating maturity payout)
	// Interest override = 5000 (5% of 100000 for 1 year)
	closeBody := fmt.Sprintf(`{
		"destination_account_id": %q,
		"interest_override": 5000,
		"penalty": 0
	}`, destID)

	rec = app.request("POST", fmt.Sprintf("/api/v1/accounts/%s/fixed-deposit/close", fdID), closeBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("close FD: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	closeResult := parseJSON(t, rec)
	closedAcct := closeResult["account"].(map[string]interface{})
	if closedAcct["is_active"] != false {
		t.Errorf("expected FD to be inactive after close, got is_active=%v", closedAcct["is_active"])
	}
	if closedAcct["balance"].(float64) != 0 {
		t.Errorf("expected FD balance 0 after close, got %v", closedAcct["balance"])
	}

	// Step 6: Verify destination received principal + interest = 105000
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", destID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dest: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	destCheck := parseJSON(t, rec)["account"].(map[string]interface{})
	if destCheck["balance"].(float64) != 105000 {
		t.Errorf("expected dest balance 105000 (principal+interest), got %v", destCheck["balance"])
	}
}

func TestFixedDepositFlow_NetWorthReflectsFD(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fdnetworth@test.com", "password123")

	// Create bank with $3000
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Bank","currency":"USD","initial_balance":300000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	bankID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Open FD for $1000
	startDate := fdFlowDate(0)
	maturityDate := fdFlowDate(365)
	fdBody := fmt.Sprintf(`{
		"name": "FD",
		"currency": "USD",
		"source_account_id": %q,
		"principal": 100000,
		"interest_rate": 5.0,
		"start_date": %q,
		"maturity_date": %q,
		"payout_type": "cumulative",
		"compounding": "annual"
	}`, bankID, startDate, maturityDate)

	rec = app.request("POST", "/api/v1/accounts/fixed-deposit", fdBody, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open FD: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Compute snapshot via pipeline - recorded_at is required.
	now := time.Now().UTC()
	recordedAt := now.Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots",
		fmt.Sprintf(`{"recorded_at":%q}`, recordedAt))
	if rec.Code != http.StatusOK {
		t.Fatalf("compute snapshot: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Query snapshots with required from_date and to_date.
	fromDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	toDate := now.AddDate(0, 0, 1).Format("2006-01-02")
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate),
		"", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get snapshots: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	snapResult := parseJSON(t, rec)
	snapData := snapResult["data"].([]interface{})
	if len(snapData) == 0 {
		t.Fatal("expected at least 1 snapshot")
	}

	snap := snapData[0].(map[string]interface{})
	// After funding FD: bank = $2000 (300000 - 100000), FD = $1000 -> total assets = $3000
	// net_worth = cash(200000) + fixed_deposit(100000) = 300000
	fixedDepositBalance := snap["fixed_deposit_balance"].(float64)
	if fixedDepositBalance != 100000 {
		t.Errorf("expected fixed_deposit_balance 100000 in snapshot, got %v", fixedDepositBalance)
	}
	totalNetWorth := snap["total_net_worth"].(float64)
	if totalNetWorth != 300000 {
		t.Errorf("expected total_net_worth 300000 (cash+fd), got %v", totalNetWorth)
	}
}

func TestFixedDepositFlow_ClosedFDExcludedFromNetWorth(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fdclosed@test.com", "password123")

	// Create bank with $2000
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Bank","currency":"USD","initial_balance":200000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	bankID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Dest account
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Destination","currency":"USD"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	destID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Open FD for $1000
	startDate := fdFlowDate(0)
	maturityDate := fdFlowDate(365)
	fdBody := fmt.Sprintf(`{
		"name": "FD",
		"currency": "USD",
		"source_account_id": %q,
		"principal": 100000,
		"interest_rate": 5.0,
		"start_date": %q,
		"maturity_date": %q,
		"payout_type": "cumulative",
		"compounding": "annual"
	}`, bankID, startDate, maturityDate)

	rec = app.request("POST", "/api/v1/accounts/fixed-deposit", fdBody, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open FD: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	fdID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Close the FD (no interest, just return principal)
	closeBody := fmt.Sprintf(`{"destination_account_id": %q, "interest_override": 0}`, destID)
	rec = app.request("POST", fmt.Sprintf("/api/v1/accounts/%s/fixed-deposit/close", fdID), closeBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("close FD: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Compute snapshot after close - recorded_at is required.
	now := time.Now().UTC()
	recordedAt := now.Format(time.RFC3339)
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots",
		fmt.Sprintf(`{"recorded_at":%q}`, recordedAt))
	if rec.Code != http.StatusOK {
		t.Fatalf("compute snapshot: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	fromDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	toDate := now.AddDate(0, 0, 1).Format("2006-01-02")
	rec = app.request("GET",
		fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate),
		"", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get snapshots: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	snapData := parseJSON(t, rec)["data"].([]interface{})
	if len(snapData) == 0 {
		t.Fatal("expected at least 1 snapshot")
	}

	snap := snapData[0].(map[string]interface{})
	fdBalance := snap["fixed_deposit_balance"].(float64)
	if fdBalance != 0 {
		t.Errorf("expected fixed_deposit_balance 0 (closed FD excluded), got %v", fdBalance)
	}
}

func TestFixedDepositFlow_ValidationErrors(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "fdvalidation@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Bank","currency":"USD","initial_balance":100000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	bankID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	t.Run("rejects_periodic_without_frequency", func(t *testing.T) {
		startDate := fdFlowDate(0)
		maturityDate := fdFlowDate(365)
		body := fmt.Sprintf(`{
			"name": "FD",
			"currency": "USD",
			"source_account_id": %q,
			"principal": 50000,
			"interest_rate": 5.0,
			"start_date": %q,
			"maturity_date": %q,
			"payout_type": "periodic",
			"compounding": "monthly"
		}`, bankID, startDate, maturityDate)

		rec := app.request("POST", "/api/v1/accounts/fixed-deposit", body, token)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for periodic without frequency, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects_insufficient_balance", func(t *testing.T) {
		startDate := fdFlowDate(0)
		maturityDate := fdFlowDate(365)
		body := fmt.Sprintf(`{
			"name": "FD",
			"currency": "USD",
			"source_account_id": %q,
			"principal": 999999999,
			"interest_rate": 5.0,
			"start_date": %q,
			"maturity_date": %q,
			"payout_type": "cumulative",
			"compounding": "annual"
		}`, bankID, startDate, maturityDate)

		rec := app.request("POST", "/api/v1/accounts/fixed-deposit", body, token)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for insufficient balance, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
