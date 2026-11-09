package integration

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

const csvStatement = `Transaction Date,Details,Withdrawal,Deposit
05/03/2024,ACME PAYROLL,,2500.00
06/03/2024,Whole Foods Market,45.10,
06/03/2024,Whole Foods Market,45.10,
07/03/2024,Coffee,3.50,
`

// csvImportRequest posts a CSV file and form fields to the CSV import endpoint.
func (app *testApp) csvImportRequest(t *testing.T, token, content string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(content))
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/imports/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	return rec
}

func TestCSVImportFlow(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "csv-a@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "csv-b@test.com", "password123")

	rec := app.request("POST", "/api/v1/accounts/cash", `{"name":"Bank","initial_balance":100000}`, token)
	acct := parseJSON(t, rec)["account"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/categories", `{"name":"Groceries","type":"expense"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category: %d %s", rec.Code, rec.Body.String())
	}
	groceries := parseJSON(t, rec)["category"].(map[string]interface{})["id"].(string)
	rec = app.request("POST", "/api/v1/rules", fmt.Sprintf(
		`{"name":"WF","conditions":[{"field":"description","operator":"contains","value_text":"whole foods"}],"actions":[{"action_type":"set_category","category_id":%q}]}`, groceries), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule: %d %s", rec.Code, rec.Body.String())
	}

	// An entry typed by hand that the statement repeats.
	app.request("POST", "/api/v1/transactions", fmt.Sprintf(
		`{"account_id":%q,"type":"expense","amount":350,"description":"coffee","date":"2024-03-07T09:00:00Z"}`, acct), token)

	// First look: columns, a sample and the suggested mapping, nothing written.
	rec = app.csvImportRequest(t, token, csvStatement, map[string]string{"account_id": acct})
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	sug := body["suggested_mapping"].(map[string]interface{})
	if sug["date_column"] != "Transaction Date" || sug["description_column"] != "Details" ||
		sug["debit_column"] != "Withdrawal" || sug["credit_column"] != "Deposit" || sug["date_format"] != "DD/MM/YYYY" {
		t.Fatalf("suggested mapping = %v", sug)
	}
	if len(body["columns"].([]interface{})) != 4 || len(body["sample"].([]interface{})) != 4 || body["summary"] != nil {
		t.Errorf("inspect body = %v", body)
	}

	mapping := map[string]string{
		"account_id": acct, "date_column": "Transaction Date", "description_column": "Details",
		"debit_column": "Withdrawal", "credit_column": "Deposit", "date_format": "DD/MM/YYYY", "decimal_separator": ".",
	}
	// Dry run with the mapping.
	rec = app.csvImportRequest(t, token, csvStatement, mapping)
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	sm := parseJSON(t, rec)["summary"].(map[string]interface{})
	if sm["would_create"] != float64(3) || sm["skip_manual"] != float64(1) || sm["categorized_by_rule"] != float64(2) {
		t.Fatalf("dry run summary = %v", sm)
	}
	if n := app.auditCount(t, "IMPORT_CSV"); n != 0 {
		t.Errorf("dry run audited %d", n)
	}

	// Apply.
	mapping["apply"] = "true"
	rec = app.csvImportRequest(t, token, csvStatement, mapping)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}
	if res := parseJSON(t, rec); res["applied"] != true || res["created"] != float64(3) {
		t.Fatalf("apply result = %v", res)
	}
	if n := app.auditCount(t, "IMPORT_CSV"); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
	rec = app.request("GET", "/api/v1/accounts/"+acct, "", token)
	if bal := parseJSON(t, rec)["account"].(map[string]interface{})["balance"].(float64); bal != 100000+250000-4510*2-350 {
		t.Errorf("balance = %.0f", bal)
	}
	rec = app.request("GET", "/api/v1/transactions?page_size=50", "", token)
	imported, withCat, coffeeID := 0, 0, ""
	for _, r := range parseJSON(t, rec)["data"].([]interface{}) {
		row := r.(map[string]interface{})
		if row["source"] != "import" {
			continue
		}
		imported++
		if row["description"] == "ACME PAYROLL" {
			coffeeID = row["id"].(string)
		}
		if row["category_id"] == groceries {
			withCat++
		}
		if ref, _ := row["import_ref"].(string); len(ref) < 10 || ref[:4] != "csv:" {
			t.Errorf("import_ref = %v", row["import_ref"])
		}
	}
	if imported != 3 || withCat != 2 {
		t.Fatalf("imported %d rows (want 3), %d categorised by the rule (want 2)", imported, withCat)
	}

	// CSV rows are plain entries: unlike IBKR rows their amount stays editable.
	if rec := app.request("PUT", "/api/v1/transactions/"+coffeeID, `{"amount":260000}`, token); rec.Code != http.StatusOK {
		t.Errorf("editing an imported CSV row: %d %s", rec.Code, rec.Body.String())
	}

	// Idempotent: the same file again creates nothing.
	rec = app.csvImportRequest(t, token, csvStatement, mapping)
	res := parseJSON(t, rec)
	if rec.Code != http.StatusOK || res["created"] != float64(0) || res["summary"].(map[string]interface{})["skip_duplicate"] != float64(3) {
		t.Errorf("re-import: %d %v", rec.Code, res)
	}

	// Scoping: user B cannot import into A's account, and sees none of A's rows.
	rec = app.csvImportRequest(t, tokenB, csvStatement, map[string]string{"account_id": acct})
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign account: %d %s", rec.Code, rec.Body.String())
	}
	rec = app.request("GET", "/api/v1/transactions", "", tokenB)
	if data := parseJSON(t, rec)["data"].([]interface{}); len(data) != 0 {
		t.Errorf("user B sees %d transactions", len(data))
	}

	// Errors: no token, bad mapping, unknown column.
	if rec := app.csvImportRequest(t, "", csvStatement, map[string]string{"account_id": acct}); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rec.Code)
	}
	bad := map[string]string{"account_id": acct, "date_column": "Nope", "description_column": "Details", "amount_column": "Details"}
	if rec := app.csvImportRequest(t, token, csvStatement, bad); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown column: %d %s", rec.Code, rec.Body.String())
	}
}
