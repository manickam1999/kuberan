package integration

import (
	"fmt"
	"net/http"
	"testing"
)

// demoAccounts returns the user's accounts by name.
func demoAccounts(t *testing.T, app *testApp, token string) map[string]map[string]interface{} {
	t.Helper()
	rec := app.request("GET", "/api/v1/accounts?page_size=100", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list accounts: %d %s", rec.Code, rec.Body.String())
	}
	out := map[string]map[string]interface{}{}
	for _, a := range parseJSON(t, rec)["data"].([]interface{}) {
		acct := a.(map[string]interface{})
		out[acct["name"].(string)] = acct
	}
	return out
}

func errorCode(body map[string]interface{}) string {
	e, ok := body["error"].(map[string]interface{})
	if !ok {
		return ""
	}
	code, _ := e["code"].(string)
	return code
}

func TestDemoFlow(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "demo-a@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "demo-b@test.com", "password123")

	// The server reports demo availability in the system status.
	st := parseJSON(t, app.request("GET", "/api/v1/system/status", "", tokenA))
	if st["demo_enabled"] != true || st["demo_loaded"] != false {
		t.Fatalf("status before load = %v", st)
	}

	rec := app.request("POST", "/api/v1/demo/load", "", tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("load: %d %s", rec.Code, rec.Body.String())
	}
	if total := parseJSON(t, rec)["total"].(float64); total < 100 {
		t.Errorf("total rows = %v", total)
	}
	if st := parseJSON(t, app.request("GET", "/api/v1/system/status", "", tokenA)); st["demo_loaded"] != true {
		t.Errorf("status after load = %v", st)
	}

	accts := demoAccounts(t, app, tokenA)
	for _, name := range []string{"DBS Everyday", "High-Yield Savings", "Maybank FD 12M", "IBKR Brokerage", "Amex Gold"} {
		if accts[name] == nil {
			t.Fatalf("missing demo account %q in %v", name, accts)
		}
	}
	if len(demoAccounts(t, app, tokenB)) != 0 {
		t.Error("user B sees user A's demo accounts")
	}
	// Analytics and holdings work on the sample data.
	if rec := app.request("GET", "/api/v1/transactions?page_size=5", "", tokenA); rec.Code != http.StatusOK {
		t.Errorf("transactions: %d", rec.Code)
	}
	if rec := app.request("GET", "/api/v1/investments/portfolio", "", tokenA); rec.Code != http.StatusOK {
		t.Errorf("portfolio: %d %s", rec.Code, rec.Body.String())
	}
	if rec := app.request("GET", "/api/v1/budgets", "", tokenA); rec.Code != http.StatusOK {
		t.Errorf("budgets: %d", rec.Code)
	}
	if rec := app.request("GET", "/api/v1/receivables/summary", "", tokenA); rec.Code != http.StatusOK {
		t.Errorf("receivables summary: %d %s", rec.Code, rec.Body.String())
	}

	// Loading twice is refused.
	rec = app.request("POST", "/api/v1/demo/load", "", tokenA)
	if rec.Code != http.StatusConflict || errorCode(parseJSON(t, rec)) != "DEMO_ALREADY_LOADED" {
		t.Errorf("second load: %d %s", rec.Code, rec.Body.String())
	}

	// A transfer between a sample and a real account is blocked; between two
	// sample accounts it works.
	rec = app.request("POST", "/api/v1/accounts/cash", `{"name":"Real wallet","initial_balance":10000}`, tokenA)
	realID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	dbs := accts["DBS Everyday"]["id"].(string)
	hys := accts["High-Yield Savings"]["id"].(string)
	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":100,"description":"x"}`, dbs, realID), tokenA)
	if rec.Code != http.StatusConflict || errorCode(parseJSON(t, rec)) != "DEMO_TRANSFER_BLOCKED" {
		t.Errorf("demo->real transfer: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":100,"description":"x"}`, realID, hys), tokenA)
	if rec.Code != http.StatusConflict {
		t.Errorf("real->demo transfer: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("POST", "/api/v1/transactions/transfer",
		fmt.Sprintf(`{"from_account_id":%q,"to_account_id":%q,"amount":100,"description":"x"}`, dbs, hys), tokenA)
	if rec.Code != http.StatusCreated {
		t.Errorf("demo->demo transfer: %d %s", rec.Code, rec.Body.String())
	}

	// What the user added inside the sample accounts goes with them; the real
	// account and its balance stay.
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":500,"description":"inside the sample"}`, dbs), tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tx on demo account: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("DELETE", "/api/v1/demo", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	left := demoAccounts(t, app, tokenA)
	if len(left) != 1 || left["Real wallet"] == nil || left["Real wallet"]["balance"].(float64) != 10000 {
		t.Errorf("accounts after removal = %v", left)
	}
}

func TestDemoFlow_RemoveCleanAndLoadBlockedByOwnData(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "demo-c@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "demo-d@test.com", "password123")

	// Own data blocks the load (opening balances do not count).
	rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Wallet","initial_balance":2000}`, tokenB)
	walletB := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	if rec := app.request("POST", "/api/v1/demo/load", "", tokenB); rec.Code != http.StatusCreated {
		t.Fatalf("load with an opening balance only: %d %s", rec.Code, rec.Body.String())
	}
	app.request("DELETE", "/api/v1/demo", "", tokenB)
	app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":300,"description":"coffee"}`, walletB), tokenB)
	rec = app.request("POST", "/api/v1/demo/load", "", tokenB)
	if rec.Code != http.StatusConflict || errorCode(parseJSON(t, rec)) != "DEMO_HAS_DATA" {
		t.Errorf("load with own transactions: %d %s", rec.Code, rec.Body.String())
	}

	// A clean load and removal leaves the user exactly as before.
	if rec := app.request("POST", "/api/v1/demo/load", "", tokenA); rec.Code != http.StatusCreated {
		t.Fatalf("load: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("DELETE", "/api/v1/demo", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	if got := len(demoAccounts(t, app, tokenA)); got != 0 {
		t.Errorf("accounts left after removal: %d", got)
	}
	rec = app.request("GET", "/api/v1/transactions?page_size=5", "", tokenA)
	if data := parseJSON(t, rec)["data"].([]interface{}); len(data) != 0 {
		t.Errorf("transactions left after removal: %d", len(data))
	}
	// Removal is idempotent and user B's own data is untouched.
	if rec := app.request("DELETE", "/api/v1/demo", "", tokenA); rec.Code != http.StatusOK {
		t.Errorf("second remove: %d", rec.Code)
	}
	if len(demoAccounts(t, app, tokenB)) != 1 {
		t.Error("user B's own account was affected")
	}
	if st := parseJSON(t, app.request("GET", "/api/v1/system/status", "", tokenA)); st["demo_loaded"] != false {
		t.Errorf("status after removal = %v", st)
	}
}
