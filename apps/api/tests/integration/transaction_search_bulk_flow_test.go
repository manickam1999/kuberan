package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"kuberan/internal/middleware"
	"kuberan/internal/models"
)

// listIDs queries GET /transactions and returns the matching rows keyed by id.
func listIDs(t *testing.T, app *testApp, token, query string) map[string]map[string]interface{} {
	t.Helper()
	rec := app.request("GET", "/api/v1/transactions?"+query, "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %q: %d %s", query, rec.Code, rec.Body.String())
	}
	out := map[string]map[string]interface{}{}
	for _, row := range parseJSON(t, rec)["data"].([]interface{}) {
		m := row.(map[string]interface{})
		out[m["id"].(string)] = m
	}
	return out
}

// TestTransactionSearchAndBulkFlow covers notes, source, search, filters and
// the bulk endpoint end to end over HTTP.
func TestTransactionSearchAndBulkFlow(t *testing.T) {
	app := setupApp(t)
	token, _, userID := app.registerUser(t, "search@test.com", "password123")

	rec := app.request("POST", "/api/v1/categories", `{"name":"Food","type":"expense"}`, token)
	foodID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/categories", `{"name":"Salary","type":"income"}`, token)
	salaryID := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)

	rec = app.request("POST", "/api/v1/accounts/cash", `{"name":"Checking","initial_balance":100000}`, token)
	accountID := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)

	create := func(body string) map[string]interface{} {
		t.Helper()
		rec := app.request("POST", "/api/v1/transactions", body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		return parseJSON(t, rec)["transaction"].(map[string]interface{})
	}

	coffee := create(fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":1250,"description":"Starbucks","notes":"morning meeting"}`, accountID))
	lunch := create(fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":6473,"description":"Lunch"}`, accountID))
	pay := create(fmt.Sprintf(`{"account_id":%q,"type":"income","amount":50000,"description":"Pay"}`, accountID))

	// Notes and source come back on the row; REST writes "web".
	if coffee["notes"] != "morning meeting" || coffee["source"] != "web" {
		t.Errorf("notes/source = %v / %v", coffee["notes"], coffee["source"])
	}

	// The bot's token carries the src claim, so its rows are "telegram".
	botToken, err := middleware.GenerateBotToken(&models.User{Base: models.Base{ID: userID}, Email: "search@test.com"})
	if err != nil {
		t.Fatal(err)
	}
	rec = app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":300,"description":"Kopi via bot"}`, accountID), botToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bot create: %d %s", rec.Code, rec.Body.String())
	}
	if src := parseJSON(t, rec)["transaction"].(map[string]interface{})["source"]; src != "telegram" {
		t.Errorf("bot source = %v", src)
	}

	// Search: description, notes, amount.
	for q, want := range map[string]string{
		"starbucks": coffee["id"].(string),
		"MEETING":   coffee["id"].(string),
		"64.73":     lunch["id"].(string),
	} {
		got := listIDs(t, app, token, "q="+url.QueryEscape(q))
		if len(got) != 1 || got[want] == nil {
			t.Errorf("q=%q: got %d rows", q, len(got))
		}
	}
	if got := listIDs(t, app, token, "q=starbucks&page_size=5"); len(got) != 1 {
		t.Errorf("page_size=5 search returned %d rows", len(got))
	}

	// Another user sees nothing.
	otherToken, _, _ := app.registerUser(t, "other@test.com", "password123")
	if got := listIDs(t, app, otherToken, "q=starbucks"); len(got) != 0 {
		t.Errorf("another user's search returned %d rows", len(got))
	}

	// Filters that match nothing here.
	if got := listIDs(t, app, token, "shared=true"); len(got) != 0 {
		t.Errorf("shared=true returned %d rows", len(got))
	}
	if got := listIDs(t, app, token, "has_attachment=true"); len(got) != 0 {
		t.Errorf("has_attachment=true returned %d rows", len(got))
	}
	if rec := app.request("GET", "/api/v1/transactions?shared=maybe", "", token); rec.Code != http.StatusBadRequest {
		t.Errorf("bad boolean: %d", rec.Code)
	}

	// Bulk: a type mismatch on the second row rolls the whole request back.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q,%q],"set":{"category_id":%q}}`, coffee["id"], pay["id"], foodID), token)
	if rec.Code != http.StatusBadRequest || parseJSON(t, rec)["error"].(map[string]interface{})["code"] != "CATEGORY_TYPE_MISMATCH" {
		t.Fatalf("mismatch: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/transactions/"+coffee["id"].(string), "", token)
	if parseJSON(t, rec)["transaction"].(map[string]interface{})["category_id"] != nil {
		t.Error("category changed despite the failed bulk request")
	}

	// An unknown id yields 404 and changes nothing.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q,"00000000-0000-0000-0000-000000000000"],"set":{"notes_append":"x"}}`, coffee["id"]), token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d %s", rec.Code, rec.Body.String())
	}

	// Another user's ids are not found.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q],"set":{"notes_append":"x"}}`, coffee["id"]), otherToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign id: %d", rec.Code)
	}

	// Success: recategorise both expenses and append a note.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q,%q],"set":{"category_id":%q,"notes_append":"reviewed"}}`, coffee["id"], lunch["id"], foodID), token)
	if rec.Code != http.StatusOK || parseJSON(t, rec)["updated"].(float64) != 2 {
		t.Fatalf("bulk: %d %s", rec.Code, rec.Body.String())
	}
	got := listIDs(t, app, token, "q=starbucks")
	row := got[coffee["id"].(string)]
	if row["category_id"] != foodID || row["notes"] != "morning meeting\nreviewed" {
		t.Errorf("coffee after bulk: category=%v notes=%q", row["category_id"], row["notes"])
	}

	// The income row cannot take an expense category but can take an income one.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q],"set":{"category_id":%q}}`, pay["id"], salaryID), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("income recategorise: %d %s", rec.Code, rec.Body.String())
	}

	// The opening balance cannot be changed in bulk.
	rec = app.request("GET", "/api/v1/transactions?type=opening_balance", "", token)
	ob := parseJSON(t, rec)["data"].([]interface{})[0].(map[string]interface{})
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q],"delete":true}`, ob["id"]), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("opening balance: %d %s", rec.Code, rec.Body.String())
	}

	// Bulk delete reverses the balances.
	rec = app.request("POST", "/api/v1/transactions/bulk",
		fmt.Sprintf(`{"ids":[%q,%q],"delete":true}`, coffee["id"], lunch["id"]), token)
	if rec.Code != http.StatusOK || parseJSON(t, rec)["deleted"].(float64) != 2 {
		t.Fatalf("bulk delete: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/accounts/"+accountID, "", token)
	balance := parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64)
	// 100000 opening + 50000 pay - 300 bot coffee.
	if balance != 149700 {
		t.Errorf("balance after bulk delete = %v, want 149700", balance)
	}

	// Validation: empty ids, and neither set nor delete.
	if rec := app.request("POST", "/api/v1/transactions/bulk", `{"ids":[]}`, token); rec.Code != http.StatusBadRequest {
		t.Errorf("empty ids: %d", rec.Code)
	}
	if rec := app.request("POST", "/api/v1/transactions/bulk", fmt.Sprintf(`{"ids":[%q]}`, pay["id"]), token); rec.Code != http.StatusBadRequest {
		t.Errorf("no change: %d", rec.Code)
	}
}
