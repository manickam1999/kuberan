package integration

import (
	"fmt"
	"net/http"
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

// TestConnectedAppsFlow covers plan 023 1.6 over HTTP: the connected-apps list
// (client_host, write_limits), the per-app and all-apps change logs, the
// "via app" name on transactions, and that none of it crosses users.
func TestConnectedAppsFlow(t *testing.T) {
	app := setupApp(t)
	tokenA, _, idA := app.registerUser(t, "apps-a@test.com", "password123")
	tokenB, _, idB := app.registerUser(t, "apps-b@test.com", "password123")

	grants := services.NewOAuthGrantService(app.DB)
	audit := services.NewAuditService(app.DB)
	const clientID = "claude-connector"

	// User A connected an app that can write; B has no grants at all.
	if _, err := grants.RecordConsent(services.RecordConsentInput{
		UserID: idA, ClientID: clientID, ClientName: "Claude",
		Approved: []string{"read:accounts", "write:transactions"},
		Reviewed: []string{"read:accounts", "write:transactions"}, ClientHost: "claude.ai",
	}); err != nil {
		t.Fatal(err)
	}

	// An account and an MCP-sourced transaction for A, plus the audit row the
	// MCP write wrapper records for it.
	rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Checking","currency":"USD","initial_balance":0}`, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	var txn models.Transaction
	txSvcRec := app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":1871,"description":"Blue Bottle"}`, accountID), tokenA)
	if txSvcRec.Code != http.StatusCreated {
		t.Fatalf("create transaction: %d %s", txSvcRec.Code, txSvcRec.Body.String())
	}
	txID := parseJSON(t, txSvcRec)["transaction"].(map[string]interface{})["id"].(string)
	if err := app.DB.First(&txn, "id = ?", txID).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.DB.Model(&txn).Updates(map[string]any{"source": "mcp", "source_client_id": clientID}).Error; err != nil {
		t.Fatal(err)
	}
	audit.Log(idA, "MCP_CREATE_TRANSACTION", "transaction", txID, "203.0.113.7",
		map[string]any{"via": "mcp", "client_id": clientID, "amount": 1871, "description": "Blue Bottle", "currency": "USD"},
		services.LogOpts{Source: models.AuditSourceMCP, ClientID: clientID})
	// User B has an MCP row for the same client id, which A must never see.
	audit.Log(idB, "MCP_CREATE_BUDGET", "budget", "", "",
		map[string]any{"name": "B secret"}, services.LogOpts{Source: models.AuditSourceMCP, ClientID: clientID})

	t.Run("connected apps list shows host and write limits", func(t *testing.T) {
		rec := app.request("GET", "/api/v1/oauth/clients", "", tokenA)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		clients := parseJSON(t, rec)["clients"].([]interface{})
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %v", clients)
		}
		c := clients[0].(map[string]interface{})
		if c["client_id"] != clientID || c["client_host"] != "claude.ai" {
			t.Errorf("unexpected client: %v", c)
		}
		limits, _ := c["write_limits"].(map[string]interface{})
		if limits["max_amount"] != float64(1_000_000) || limits["per_minute"] != float64(30) ||
			limits["per_day"] != float64(500) || limits["currency"] == "" || limits["currency"] == nil {
			t.Errorf("unexpected write_limits: %v", limits)
		}
	})

	t.Run("per-app activity lists the change with a summary", func(t *testing.T) {
		rec := app.request("GET", "/api/v1/oauth/clients/"+clientID+"/activity", "", tokenA)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := parseJSON(t, rec)
		data := body["data"].([]interface{})
		if len(data) != 1 || body["total_items"] != float64(1) {
			t.Fatalf("expected one row, got %v", body)
		}
		row := data[0].(map[string]interface{})
		if row["action"] != "MCP_CREATE_TRANSACTION" || row["resource_type"] != "transaction" ||
			row["resource_id"] != txID || row["summary"] != "Added transaction · Blue Bottle · USD 18.71" {
			t.Errorf("unexpected row: %v", row)
		}
	})

	t.Run("all-apps activity adds the app", func(t *testing.T) {
		rec := app.request("GET", "/api/v1/oauth/clients/activity?page=1&page_size=10", "", tokenA)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		data := parseJSON(t, rec)["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("expected one row, got %v", data)
		}
		row := data[0].(map[string]interface{})
		if row["client_id"] != clientID || row["client_name"] != "Claude" {
			t.Errorf("unexpected row: %v", row)
		}
	})

	t.Run("transactions carry source_client_name", func(t *testing.T) {
		rec := app.request("GET", "/api/v1/transactions", "", tokenA)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		data := parseJSON(t, rec)["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(data))
		}
		tx := data[0].(map[string]interface{})
		if tx["source"] != "mcp" || tx["source_client_id"] != clientID || tx["source_client_name"] != "Claude" {
			t.Errorf("unexpected transaction: %v", tx)
		}
	})

	t.Run("source_client_name falls back once the grant is gone", func(t *testing.T) {
		if err := app.DB.Where("user_id = ?", idA).Delete(&models.OAuthClientGrant{}).Error; err != nil {
			t.Fatal(err)
		}
		rec := app.request("GET", "/api/v1/transactions/"+txID, "", tokenA)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		tx := parseJSON(t, rec)["transaction"].(map[string]interface{})
		if tx["source_client_name"] != "an app" {
			t.Errorf("expected the fallback, got %v", tx["source_client_name"])
		}
	})

	t.Run("user B sees nothing of A's, even with A's client id", func(t *testing.T) {
		rec := app.request("GET", "/api/v1/oauth/clients", "", tokenB)
		if got := parseJSON(t, rec)["clients"].([]interface{}); len(got) != 0 {
			t.Errorf("B must have no connected apps, got %v", got)
		}

		rec = app.request("GET", "/api/v1/oauth/clients/"+clientID+"/activity", "", tokenB)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		data := parseJSON(t, rec)["data"].([]interface{})
		if len(data) != 1 || data[0].(map[string]interface{})["summary"] != "Created budget · B secret" {
			t.Fatalf("B must see only their own row, got %v", data)
		}

		// A guessed client id that only A has used yields nothing for B.
		rec = app.request("GET", "/api/v1/oauth/clients/unknown-client/activity", "", tokenB)
		if got := parseJSON(t, rec)["data"].([]interface{}); len(got) != 0 {
			t.Errorf("a guessed client id must return nothing, got %v", got)
		}

		rec = app.request("GET", "/api/v1/transactions/"+txID, "", tokenB)
		if rec.Code != http.StatusNotFound {
			t.Errorf("B must not read A's transaction, got %d", rec.Code)
		}
	})

	t.Run("activity needs authentication", func(t *testing.T) {
		for _, path := range []string{"/api/v1/oauth/clients/activity", "/api/v1/oauth/clients/" + clientID + "/activity"} {
			if rec := app.request("GET", path, "", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401, got %d", path, rec.Code)
			}
		}
	})
}
