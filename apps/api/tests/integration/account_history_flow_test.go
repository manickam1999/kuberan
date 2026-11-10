package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// createCashAccount creates a MYR cash account and returns its ID.
func createCashAccountMYR(t *testing.T, app *testApp, token, name string, balance int64) string {
	t.Helper()
	rec := app.request("POST", "/api/v1/accounts/cash",
		fmt.Sprintf(`{"name":%q,"initial_balance":%d,"currency":"MYR"}`, name, balance), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %s", rec.Code, rec.Body.String())
	}
	return parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
}

func TestAccountHistoryFlow_PipelineWritesAndEndpointsAreScoped(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "hist-a@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "hist-b@test.com", "password123")
	accountA := createCashAccountMYR(t, app, tokenA, "A cash", 500000)
	accountB := createCashAccountMYR(t, app, tokenB, "B cash", 123)

	// A backdated expense gives the replay a multi-day history to rebuild.
	threeDaysAgo := time.Now().UTC().AddDate(0, 0, -3)
	rec := app.request("POST", "/api/v1/transactions",
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":1000,"description":"old","date":%q}`,
			accountA, threeDaysAgo.Format(time.RFC3339)), tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expense: %d %s", rec.Code, rec.Body.String())
	}

	// Nothing is recorded until the pipeline runs.
	rec = app.request("GET", "/api/v1/accounts/"+accountA+"/balance-history", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("history before pipeline: %d %s", rec.Code, rec.Body.String())
	}
	if pts := parseJSON(t, rec)["data"].(map[string]interface{})["points"].([]interface{}); len(pts) != 0 {
		t.Fatalf("expected no points before the pipeline, got %d", len(pts))
	}

	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots",
		fmt.Sprintf(`{"recorded_at":%q}`, time.Now().UTC().Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("pipeline snapshots: %d %s", rec.Code, rec.Body.String())
	}

	rec = app.request("GET", "/api/v1/accounts/"+accountA+"/balance-history", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d %s", rec.Code, rec.Body.String())
	}
	data := parseJSON(t, rec)["data"].(map[string]interface{})
	points := data["points"].([]interface{})
	if len(points) < 4 || data["currency"] != "MYR" {
		t.Fatalf("expected at least 4 daily points in MYR, got %d (%v)", len(points), data)
	}
	last := points[len(points)-1].(map[string]interface{})
	if last["day"] != time.Now().UTC().Format("2006-01-02") || last["balance"].(float64) != 499000 {
		t.Errorf("today's point = %v, want 499000", last)
	}

	// Ranged request.
	from := time.Now().UTC().Format("2006-01-02")
	rec = app.request("GET", "/api/v1/accounts/"+accountA+"/balance-history?from="+from, "", tokenA)
	if pts := parseJSON(t, rec)["data"].(map[string]interface{})["points"].([]interface{}); len(pts) != 1 {
		t.Errorf("ranged points = %d, want 1", len(pts))
	}

	// User B can't read A's history and sees only their own sparkline.
	rec = app.request("GET", "/api/v1/accounts/"+accountA+"/balance-history", "", tokenB)
	if rec.Code != http.StatusNotFound {
		t.Errorf("cross-user history = %d, want 404", rec.Code)
	}
	rec = app.request("GET", "/api/v1/accounts/sparklines?days=7", "", tokenB)
	if rec.Code != http.StatusOK {
		t.Fatalf("sparklines: %d %s", rec.Code, rec.Body.String())
	}
	items := parseJSON(t, rec)["data"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["account_id"] != accountB {
		t.Fatalf("user B sparklines = %v", items)
	}
	pts := items[0].(map[string]interface{})["points"].([]interface{})
	if len(pts) != 1 || pts[0].(map[string]interface{})["balance"].(float64) != 123 {
		t.Errorf("user B points = %v", pts)
	}

	// Validation and auth.
	if rec = app.request("GET", "/api/v1/accounts/sparklines?days=1", "", tokenA); rec.Code != http.StatusBadRequest {
		t.Errorf("days=1 = %d, want 400", rec.Code)
	}
	if rec = app.request("GET", "/api/v1/accounts/sparklines", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated sparklines = %d, want 401", rec.Code)
	}
}

func TestAnalyticsTypicalFlow_ScopedAndShaped(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "typ-a@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "typ-b@test.com", "password123")
	accountA := createCashAccountMYR(t, app, tokenA, "A cash", 0)
	createCashAccountMYR(t, app, tokenB, "B cash", 0)

	now := time.Now().UTC()
	lastMonth := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC).AddDate(0, -1, 4)
	for _, body := range []string{
		fmt.Sprintf(`{"account_id":%q,"type":"income","amount":300000,"description":"pay","date":%q}`, accountA, lastMonth.Format(time.RFC3339)),
		fmt.Sprintf(`{"account_id":%q,"type":"expense","amount":120000,"description":"rent","date":%q}`, accountA, lastMonth.Format(time.RFC3339)),
	} {
		if rec := app.request("POST", "/api/v1/transactions", body, tokenA); rec.Code != http.StatusCreated {
			t.Fatalf("create transaction: %d %s", rec.Code, rec.Body.String())
		}
	}

	rec := app.request("GET", "/api/v1/analytics/typical?months=3", "", tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("typical: %d %s", rec.Code, rec.Body.String())
	}
	d := parseJSON(t, rec)["data"].(map[string]interface{})
	if d["currency"] != "MYR" || d["months_used"].(float64) != 1 || d["avg_income"].(float64) != 300000 ||
		d["avg_spending"].(float64) != 120000 || d["saved_percent"].(float64) != 60 {
		t.Errorf("typical = %v", d)
	}
	if len(d["daily_cumulative"].([]interface{})) != 31 || len(d["by_category"].([]interface{})) != 1 {
		t.Errorf("curve/categories = %v", d)
	}

	// User B has no history: zeros, not A's figures.
	rec = app.request("GET", "/api/v1/analytics/typical", "", tokenB)
	if rec.Code != http.StatusOK {
		t.Fatalf("typical B: %d %s", rec.Code, rec.Body.String())
	}
	d = parseJSON(t, rec)["data"].(map[string]interface{})
	if d["months_used"].(float64) != 0 || d["avg_spending"].(float64) != 0 {
		t.Errorf("user B typical = %v", d)
	}

	if rec = app.request("GET", "/api/v1/analytics/typical?months=0", "", tokenA); rec.Code != http.StatusBadRequest {
		t.Errorf("months=0 = %d, want 400", rec.Code)
	}
	if rec = app.request("GET", "/api/v1/analytics/typical", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated = %d, want 401", rec.Code)
	}
}
