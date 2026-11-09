package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"kuberan/internal/models"
)

// TestRuleStatsAndSuggestions covers match counting, GET /rules/suggestions
// (including per-user scoping) and the old/new values in UPDATE_TRANSACTION.
func TestRuleStatsAndSuggestions(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "rule-stats-a@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "rule-stats-b@test.com", "password123")

	rec := app.request("POST", "/api/v1/categories", `{"name":"Coffee","type":"expense"}`, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category: %d %s", rec.Code, rec.Body.String())
	}
	coffeeID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/categories", `{"name":"Other","type":"expense"}`, tokenA)
	otherID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash", `{"name":"Checking","initial_balance":100000}`, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	// Three categorized "Kopi Tiam" rows make a suggestion.
	var lastTx string
	for i := 0; i < 3; i++ {
		rec = app.request("POST", "/api/v1/transactions", fmt.Sprintf(
			`{"account_id":%q,"category_id":%q,"type":"expense","amount":900,"description":"Kopi  Tiam"}`, accountID, coffeeID), tokenA)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create tx: %d %s", rec.Code, rec.Body.String())
		}
		lastTx = parseJSON(t, rec)["transaction"].(map[string]interface{})["id"].(string)
	}

	rec = app.request("GET", "/api/v1/rules/suggestions", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("suggestions: %d %s", rec.Code, rec.Body.String())
	}
	list := parseJSON(t, rec)["suggestions"].([]interface{})
	if len(list) != 1 {
		t.Fatalf("expected 1 suggestion, got %v", list)
	}
	s := list[0].(map[string]interface{})
	if s["description_pattern"] != "kopi tiam" || s["category_id"] != coffeeID || s["count"].(float64) != 3 || s["type"] != "expense" {
		t.Errorf("unexpected suggestion %v", s)
	}

	// User B never sees user A's suggestions.
	rec = app.request("GET", "/api/v1/rules/suggestions", "", tokenB)
	if rec.Code != http.StatusOK {
		t.Fatalf("suggestions B: %d %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["suggestions"].([]interface{}); len(got) != 0 {
		t.Errorf("user B must see no suggestions, got %v", got)
	}
	rec = app.request("GET", "/api/v1/rules/suggestions", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a token, got %d", rec.Code)
	}

	// Creating the rule removes the suggestion; the next matching create counts.
	rec = app.request("POST", "/api/v1/rules", fmt.Sprintf(
		`{"name":"Kopi","conditions":[{"field":"description","operator":"contains","value_text":"kopi"}],"actions":[{"action_type":"set_category","category_id":%q}]}`, coffeeID), tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule: %d %s", rec.Code, rec.Body.String())
	}
	ruleID := parseJSON(t, rec)["rule"].(map[string]interface{})["id"].(string)
	rec = app.request("GET", "/api/v1/rules/suggestions", "", tokenA)
	if got := parseJSON(t, rec)["suggestions"].([]interface{}); len(got) != 0 {
		t.Errorf("rule should suppress the suggestion, got %v", got)
	}

	rec = app.request("POST", "/api/v1/transactions", fmt.Sprintf(
		`{"account_id":%q,"type":"expense","amount":900,"description":"Kopi Tiam KLCC"}`, accountID), tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create matching tx: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", fmt.Sprintf("/api/v1/rules/%s", ruleID), "", tokenA)
	rule := parseJSON(t, rec)["rule"].(map[string]interface{})
	if rule["match_count"].(float64) != 1 || rule["last_matched_at"] == nil {
		t.Errorf("expected match_count 1 with last_matched_at, got %v / %v", rule["match_count"], rule["last_matched_at"])
	}

	// REST updates now record old and new values.
	rec = app.request("PUT", fmt.Sprintf("/api/v1/transactions/%s", lastTx), fmt.Sprintf(
		`{"amount":1200,"category_id":%q}`, otherID), tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("update tx: %d %s", rec.Code, rec.Body.String())
	}
	var entry models.AuditLog
	if err := app.DB.Where("action = ? AND resource_id = ?", "UPDATE_TRANSACTION", lastTx).First(&entry).Error; err != nil {
		t.Fatalf("audit row: %v", err)
	}
	changes := entry.Changes
	for _, want := range []string{`"amount"`, `"from":900`, `"to":1200`, otherID, coffeeID} {
		if !strings.Contains(changes, want) {
			t.Errorf("audit changes %s missing %s", changes, want)
		}
	}
}
