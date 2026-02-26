package integration

import (
	"fmt"
	"net/http"
	"testing"

	"kuberan/internal/models"
)

func TestAccountFlow_CreateWithInitialBalanceAndTransactions(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "acct@test.com", "password123")

	// Step 1: Create account with initial balance of $100.00 (10000 cents)
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Savings","currency":"USD","initial_balance":10000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	account := result["account"].(map[string]interface{})
	accountID := account["id"].(string)
	if account["balance"].(float64) != 10000 {
		t.Errorf("expected initial balance 10000, got %v", account["balance"])
	}

	// Step 2: Verify initial transaction exists
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s/transactions", accountID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	txResult := parseJSON(t, rec)
	totalItems := txResult["total_items"].(float64)
	if totalItems != 1 {
		t.Fatalf("expected 1 initial transaction, got %.0f", totalItems)
	}
	txData := txResult["data"].([]interface{})
	initialTx := txData[0].(map[string]interface{})
	if initialTx["type"] != string(models.TransactionTypeIncome) {
		t.Errorf("expected initial tx type 'income', got %v", initialTx["type"])
	}
	if initialTx["amount"].(float64) != 10000 {
		t.Errorf("expected initial tx amount 10000, got %v", initialTx["amount"])
	}

	// Step 3: Create income of $50.00
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"income","amount":5000,"description":"Salary"}`, accountID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 4: Create expense of $30.00
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":3000,"description":"Groceries"}`, accountID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Step 5: Verify final balance = 10000 + 5000 - 3000 = 12000
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", accountID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	acctResult := parseJSON(t, rec)
	acct := acctResult["account"].(map[string]interface{})
	finalBalance := acct["balance"].(float64)
	if finalBalance != 12000 {
		t.Errorf("expected final balance 12000, got %.0f", finalBalance)
	}

	// Step 6: Verify 3 transactions total
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s/transactions", accountID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	txResult = parseJSON(t, rec)
	if txResult["total_items"].(float64) != 3 {
		t.Errorf("expected 3 transactions, got %.0f", txResult["total_items"].(float64))
	}
}

func TestAccountFlow_CreateWithZeroBalance(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "zero@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Checking","currency":"USD"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	account := result["account"].(map[string]interface{})
	if account["balance"].(float64) != 0 {
		t.Errorf("expected balance 0, got %v", account["balance"])
	}

	// No initial transaction should exist
	accountID := account["id"].(string)
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s/transactions", accountID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	txResult := parseJSON(t, rec)
	if txResult["total_items"].(float64) != 0 {
		t.Errorf("expected 0 transactions, got %.0f", txResult["total_items"].(float64))
	}
}

func TestAccountFlow_ListAccounts(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "list@test.com", "password123")

	// Create 2 accounts
	app.request("POST", "/api/v1/accounts/cash", `{"name":"Account A"}`, token)
	app.request("POST", "/api/v1/accounts/cash", `{"name":"Account B"}`, token)

	rec := app.request("GET", "/api/v1/accounts", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	if result["total_items"].(float64) != 2 {
		t.Errorf("expected 2 accounts, got %.0f", result["total_items"].(float64))
	}
}

func TestAccountFlow_DeleteTransactionReversesBalance(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "delrev@test.com", "password123")

	// Create account with $100
	rec := app.request("POST", "/api/v1/accounts/cash",
		`{"name":"Delete Test","initial_balance":10000}`, token)
	result := parseJSON(t, rec)
	account := result["account"].(map[string]interface{})
	accountID := account["id"].(string)

	// Add expense of $30
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":3000}`, accountID), token)
	txResult := parseJSON(t, rec)
	tx := txResult["transaction"].(map[string]interface{})
	txID := tx["id"].(string)

	// Verify balance is $70
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", accountID), "", token)
	acct := parseJSON(t, rec)["account"].(map[string]interface{})
	if acct["balance"].(float64) != 7000 {
		t.Fatalf("expected 7000 after expense, got %.0f", acct["balance"].(float64))
	}

	// Delete the expense transaction
	rec = app.request("DELETE", fmt.Sprintf("/api/v1/transactions/%s", txID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", rec.Code, rec.Body.String())
	}

	// Balance should be restored to $100
	rec = app.request("GET", fmt.Sprintf("/api/v1/accounts/%s", accountID), "", token)
	acct = parseJSON(t, rec)["account"].(map[string]interface{})
	if acct["balance"].(float64) != 10000 {
		t.Errorf("expected 10000 after delete, got %.0f", acct["balance"].(float64))
	}
}
