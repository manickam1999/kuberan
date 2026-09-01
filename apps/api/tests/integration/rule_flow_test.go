package integration

import (
	"fmt"
	"net/http"
	"testing"
)

// TestRuleFlow_CreateResolveAndBackfill exercises the full rules lifecycle over
// HTTP: create a rule, have a new transaction auto-categorize, preview matches,
// and backfill an existing uncategorized transaction.
func TestRuleFlow_CreateResolveAndBackfill(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "rules@test.com", "password123")

	// Expense category to categorize into.
	rec := app.request("POST", "/api/v1/categories", `{"name":"Transport","type":"expense"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category: %d %s", rec.Code, rec.Body.String())
	}
	categoryID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	// Cash account.
	rec = app.request("POST", "/api/v1/accounts/cash", `{"name":"Checking","initial_balance":100000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// A pre-existing uncategorized "GRAB" transaction (for backfill later).
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":1500,"description":"GRAB RIDE OLD"}`, accountID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create pre-existing tx: %d %s", rec.Code, rec.Body.String())
	}
	oldTxID := parseJSON(t, rec)["transaction"].(map[string]interface{})["id"].(string)

	// Create the rule: description contains "grab" -> Transport.
	rec = app.request("POST", "/api/v1/rules",
		fmt.Sprintf(`{"name":"Grab","conditions":[{"field":"description","operator":"contains","value_text":"grab"}],"actions":[{"action_type":"set_category","category_id":%q}]}`, categoryID),
		token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule: %d %s", rec.Code, rec.Body.String())
	}
	ruleID := parseJSON(t, rec)["rule"].(map[string]interface{})["id"].(string)

	// A NEW matching transaction must auto-categorize on create.
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":2500,"description":"GRAB *RIDE KL"}`, accountID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create new tx: %d %s", rec.Code, rec.Body.String())
	}
	newTx := parseJSON(t, rec)["transaction"].(map[string]interface{})
	if cid, ok := newTx["category_id"].(string); !ok || cid != categoryID {
		t.Errorf("expected new tx auto-categorized to %s, got %v", categoryID, newTx["category_id"])
	}

	// Preview should report the pre-existing uncategorized GRAB transaction.
	rec = app.request("POST", "/api/v1/rules/preview",
		`{"conditions":[{"field":"description","operator":"contains","value_text":"grab"}]}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	// Both GRAB transactions match (old uncategorized + new categorized).
	if count := parseJSON(t, rec)["count"].(float64); count != 2 {
		t.Errorf("expected preview count 2, got %.0f", count)
	}

	// Dry-run backfill over uncategorized should find exactly the old one.
	rec = app.request("POST", fmt.Sprintf("/api/v1/rules/%s/apply", ruleID),
		`{"scope":"uncategorized","dry_run":true}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply dry-run: %d %s", rec.Code, rec.Body.String())
	}
	dry := parseJSON(t, rec)
	if dry["count"].(float64) != 1 || dry["applied"].(float64) != 0 {
		t.Errorf("expected dry-run count=1 applied=0, got count=%.0f applied=%.0f", dry["count"].(float64), dry["applied"].(float64))
	}

	// Commit the backfill.
	rec = app.request("POST", fmt.Sprintf("/api/v1/rules/%s/apply", ruleID),
		`{"scope":"uncategorized","dry_run":false}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply commit: %d %s", rec.Code, rec.Body.String())
	}
	if parseJSON(t, rec)["applied"].(float64) != 1 {
		t.Errorf("expected applied=1 on commit")
	}

	// The old transaction is now categorized.
	rec = app.request("GET", fmt.Sprintf("/api/v1/transactions/%s", oldTxID), "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get old tx: %d %s", rec.Code, rec.Body.String())
	}
	got := parseJSON(t, rec)["transaction"].(map[string]interface{})
	if cid, ok := got["category_id"].(string); !ok || cid != categoryID {
		t.Errorf("expected backfilled category %s on old tx, got %v", categoryID, got["category_id"])
	}
}
