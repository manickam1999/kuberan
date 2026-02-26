package integration

import (
	"fmt"
	"net/http"
	"testing"
)

func TestTransferFlow_SuccessfulTransfer(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "xfer@test.com", "password123")

	// Create account A with $200
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Account A","initial_balance":20000}`, token)
	acctA := parseJSON(t, rec)["account"].(map[string]interface{})
	acctAID := acctA["id"].(string)

	// Create account B with $50
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Account B","initial_balance":5000}`, token)
	acctB := parseJSON(t, rec)["account"].(map[string]interface{})
	acctBID := acctB["id"].(string)

	// Transfer $75 from A to B
	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":7500,"description":"Rent money"}`,
			acctAID, acctBID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	xferResult := parseJSON(t, rec)
	xferTx := xferResult["transaction"].(map[string]interface{})
	xferID := xferTx["id"].(string)

	// Verify A balance: 20000 - 7500 = 12500
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctAID), "", token)
	acctAResult := parseJSON(t, rec)["account"].(map[string]interface{})
	if acctAResult["balance"].(float64) != 12500 {
		t.Errorf("expected account A balance 12500, got %.0f", acctAResult["balance"].(float64))
	}

	// Verify B balance: 5000 + 7500 = 12500
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctBID), "", token)
	acctBResult := parseJSON(t, rec)["account"].(map[string]interface{})
	if acctBResult["balance"].(float64) != 12500 {
		t.Errorf("expected account B balance 12500, got %.0f", acctBResult["balance"].(float64))
	}

	// Delete the transfer
	rec = app.request("DELETE", fmt.Sprintf("/api/v1/transactions/%s", xferID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify A balance restored to 20000
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctAID), "", token)
	acctAResult = parseJSON(t, rec)["account"].(map[string]interface{})
	if acctAResult["balance"].(float64) != 20000 {
		t.Errorf("expected account A balance 20000 after delete, got %.0f", acctAResult["balance"].(float64))
	}

	// Verify B balance restored to 5000
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctBID), "", token)
	acctBResult = parseJSON(t, rec)["account"].(map[string]interface{})
	if acctBResult["balance"].(float64) != 5000 {
		t.Errorf("expected account B balance 5000 after delete, got %.0f", acctBResult["balance"].(float64))
	}
}

func TestTransferFlow_SameAccountRejected(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "same@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Only Account","initial_balance":10000}`, token)
	acct := parseJSON(t, rec)["account"].(map[string]interface{})
	acctID := acct["id"].(string)

	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":1000}`,
			acctID, acctID), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	errObj := result["error"].(map[string]interface{})
	if errObj["code"] != "SAME_ACCOUNT_TRANSFER" {
		t.Errorf("expected SAME_ACCOUNT_TRANSFER, got %v", errObj["code"])
	}
}

func TestTransferFlow_InsufficientBalance(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "insuf@test.com", "password123")

	// Account A with $10
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Poor Account","initial_balance":1000}`, token)
	acctA := parseJSON(t, rec)["account"].(map[string]interface{})
	acctAID := acctA["id"].(string)

	// Account B
	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Rich Account","initial_balance":0}`, token)
	acctB := parseJSON(t, rec)["account"].(map[string]interface{})
	acctBID := acctB["id"].(string)

	// Try to transfer $50 from A ($10)
	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":5000}`,
			acctAID, acctBID), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	errObj := result["error"].(map[string]interface{})
	if errObj["code"] != "INSUFFICIENT_BALANCE" {
		t.Errorf("expected INSUFFICIENT_BALANCE, got %v", errObj["code"])
	}

	// Verify A balance unchanged
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctAID), "", token)
	acctAResult := parseJSON(t, rec)["account"].(map[string]interface{})
	if acctAResult["balance"].(float64) != 1000 {
		t.Errorf("expected balance 1000 unchanged, got %.0f", acctAResult["balance"].(float64))
	}
}

func TestTransferFlow_MultipleTransfers(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "multi@test.com", "password123")

	// Create 3 accounts
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"A","initial_balance":10000}`, token)
	acctA := parseJSON(t, rec)["account"].(map[string]interface{})
	acctAID := acctA["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"B","initial_balance":5000}`, token)
	acctB := parseJSON(t, rec)["account"].(map[string]interface{})
	acctBID := acctB["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash",
		`{"name":"C","initial_balance":0}`, token)
	acctC := parseJSON(t, rec)["account"].(map[string]interface{})
	acctCID := acctC["id"].(string)

	// A -> B: $30
	app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":3000}`, acctAID, acctBID), token)

	// B -> C: $60
	app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":6000}`, acctBID, acctCID), token)

	// Verify: A=7000, B=2000 (5000+3000-6000), C=6000
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctAID), "", token)
	if parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64) != 7000 {
		t.Error("expected A=7000")
	}

	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctBID), "", token)
	if parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64) != 2000 {
		t.Error("expected B=2000")
	}

	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", acctCID), "", token)
	if parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64) != 6000 {
		t.Error("expected C=6000")
	}
}
