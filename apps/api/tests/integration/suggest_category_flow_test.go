package integration

import (
	"fmt"
	"net/http"
	"testing"
)

// TestSuggestCategoryFlow covers POST /transactions/suggest-category: history,
// rule precedence, validation and user scoping.
func TestSuggestCategoryFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "suggest@test.com", "password123")
	otherToken, _, _ := app.registerUser(t, "suggest-other@test.com", "password123")

	rec := app.request("POST", "/api/v1/categories", `{"name":"Dining","type":"expense"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category: %d %s", rec.Code, rec.Body.String())
	}
	diningID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/accounts/cash", `{"name":"Checking","initial_balance":100000}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	suggest := func(tok, body string) (int, map[string]interface{}) {
		r := app.request("POST", "/api/v1/transactions/suggest-category", body, tok)
		if r.Code != http.StatusOK {
			return r.Code, nil
		}
		return r.Code, parseJSON(t, r)
	}

	if code, out := suggest(token, `{"description":"coffee","type":"expense"}`); code != http.StatusOK || out["category_id"] != nil {
		t.Fatalf("expected null suggestion, got %d %v", code, out)
	}

	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"category_id":%q,"type":"expense","amount":1840,"description":"Coffee"}`, accountID, diningID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tx: %d %s", rec.Code, rec.Body.String())
	}

	code, out := suggest(token, `{"description":"coffee","type":"expense"}`)
	if code != http.StatusOK || out["category_id"] != diningID || out["reason"] != "history" || out["category_name"] != "Dining" {
		t.Fatalf("expected history suggestion, got %d %v", code, out)
	}

	// Another user never sees it.
	if code, out := suggest(otherToken, `{"description":"coffee","type":"expense"}`); code != http.StatusOK || out["category_id"] != nil {
		t.Fatalf("history leaked across users: %d %v", code, out)
	}

	// A rule takes precedence.
	rec = app.request("POST", "/api/v1/categories", `{"name":"Cafes","type":"expense"}`, token)
	cafesID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/rules",
		fmt.Sprintf(`{"name":"Cafe","conditions":[{"field":"description","operator":"contains","value_text":"coffee"}],"actions":[{"action_type":"set_category","category_id":%q}]}`, cafesID), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule: %d %s", rec.Code, rec.Body.String())
	}
	code, out = suggest(token, `{"description":"coffee","type":"expense"}`)
	if code != http.StatusOK || out["category_id"] != cafesID || out["reason"] != "rule" {
		t.Fatalf("expected rule suggestion, got %d %v", code, out)
	}

	// Validation.
	for _, body := range []string{
		`{"description":"x","type":"transfer"}`,
		`{"description":"x"}`,
		`{"type":"expense"}`,
	} {
		if code, _ := suggest(token, body); code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", body, code)
		}
	}
	if r := app.request("POST", "/api/v1/transactions/suggest-category", `{"description":"x","type":"expense"}`, ""); r.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a token, got %d", r.Code)
	}
}
