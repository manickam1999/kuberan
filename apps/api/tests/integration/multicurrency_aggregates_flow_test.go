package integration

import (
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"
)

// TestMultiCurrencyAggregatesFlow covers plan 024 phase 4 end to end over HTTP
// for a MYR user. USD expenses stay in USD on the account and in the budget,
// while the MYR aggregates convert each expense at its own day's rate. A USD
// receivable settled into the MYR account is credited at the stated amount and
// reversed exactly when the settlement is deleted.
//
// Inputs (all in the current month, UTC):
//   - rates USD->MYR: 4.00 on D1 (the 1st), 5.00 on D2 (the 2nd), 4.50 now
//   - USD wallet opens at US$1000.00; MYR account opens at RM 10,000.00
//   - USD expense US$10.00 on D1, US$20.00 on D2 (category "Travel")
//   - MYR expense RM 30.00 on D1 (same category)
//   - shared claim US$8.00 on the USD wallet, owed in full by Dana (personal share 0)
//   - settle US$8.00 into the MYR account at RM 36.00 (8 x 4.50)
//
// D1 and D2 are the 1st and 2nd rather than the "e.g." days of last month
// because monthly-summary and budget progress only read the current period. D2
// must end before the "now" rate is recorded, since rates resolve to the latest
// observation at or before the end of a day, so the test needs today to be the
// 3rd (UTC) or later.
func TestMultiCurrencyAggregatesFlow(t *testing.T) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if now.Before(monthStart.AddDate(0, 0, 2)) {
		t.Skip("needs two past days in the current month: run on or after the 3rd (UTC)")
	}
	d1 := monthStart.Add(12 * time.Hour)
	d2 := monthStart.AddDate(0, 0, 1).Add(12 * time.Hour)

	app := setupApp(t)
	token, _, _ := app.registerUser(t, "aggregates@test.com", "password123") // base currency defaults to MYR

	// Rates: 4.00 on D1, 5.00 on D2, 4.50 "now". The 4.50 is stamped at the start
	// of today (UTC) so that a settlement dated today, which resolves its rate at
	// that date, can see it; it is still after D2 ends.
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	rec := app.pipelineRequest("POST", "/api/v1/pipeline/fx-rates", fmt.Sprintf(
		`{"rates":[{"base":"USD","quote":"MYR","rate":"4.00","recorded_at":%q},{"base":"USD","quote":"MYR","rate":"5.00","recorded_at":%q},{"base":"USD","quote":"MYR","rate":"4.50","recorded_at":%q}]}`,
		d1.Format(time.RFC3339), d2.Format(time.RFC3339), todayStart.Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("post rates: %d %s", rec.Code, rec.Body.String())
	}

	createOK := func(path, body string) map[string]interface{} {
		t.Helper()
		rec := app.request("POST", path, body, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s %s: %d %s", path, body, rec.Code, rec.Body.String())
		}
		return parseJSON(t, rec)
	}
	getJSON := func(path string) map[string]interface{} {
		t.Helper()
		rec := app.request("GET", path, "", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
		return parseJSON(t, rec)
	}
	approx := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

	// 1. Accounts. Initial balances are in each account's own currency.
	myrCash := createOK("/api/v1/accounts/cash", `{"name":"Maybank","currency":"MYR","initial_balance":1000000}`)["account"].(map[string]interface{})["id"].(string)
	usdCash := createOK("/api/v1/accounts/cash", `{"name":"USD wallet","currency":"USD","initial_balance":100000}`)["account"].(map[string]interface{})["id"].(string)

	// 2. Category, expenses.
	cat := createOK("/api/v1/categories", `{"name":"Travel","type":"expense"}`)["category"].(map[string]interface{})["id"].(string)
	expense := func(accountID string, amount int64, at time.Time, desc string) {
		t.Helper()
		createOK("/api/v1/transactions", fmt.Sprintf(
			`{"account_id":%q,"type":"expense","amount":%d,"category_id":%q,"description":%q,"date":%q}`,
			accountID, amount, cat, desc, at.Format(time.RFC3339)))
	}
	expense(usdCash, 1000, d1, "USD taxi")   // US$10.00 on D1
	expense(usdCash, 2000, d2, "USD dinner") // US$20.00 on D2
	expense(myrCash, 3000, d1, "MYR lunch")  // RM 30.00 on D1

	// 3. Budgets. The API allows one active budget per (category, period), so the
	// MYR budget is yearly; it covers the same expenses and the same month.
	usdBudget := createOK("/api/v1/budgets", fmt.Sprintf(
		`{"category_id":%q,"name":"Travel USD","amount":5000,"period":"monthly","currency":"USD"}`, cat))["budget"].(map[string]interface{})["id"].(string)
	myrBudget := createOK("/api/v1/budgets", fmt.Sprintf(
		`{"category_id":%q,"name":"Travel MYR","amount":50000,"period":"yearly","currency":"MYR"}`, cat))["budget"].(map[string]interface{})["id"].(string)

	// 4. Shared claim: US$8.00 on the USD wallet, owed in full by Dana. Its
	// personal share is 800 - 800 = 0, so it adds nothing to spending or budgets,
	// but the wallet is debited the full US$8.00.
	shared := createOK("/api/v1/transactions/shared", fmt.Sprintf(
		`{"account_id":%q,"category_id":%q,"amount":800,"description":"Shared hotel","date":%q,"kind":"claim","mode":"custom","counterparties":[{"name":"Dana","amount":800}]}`,
		usdCash, cat, today()))["transaction"].(map[string]interface{})
	if num(shared, "personal_amount") != 0 {
		t.Fatalf("claim personal_amount = %v, want 0", shared["personal_amount"])
	}
	recs := shared["receivables"].([]interface{})
	if len(recs) != 1 {
		t.Fatalf("receivables = %v", recs)
	}
	recID := recs[0].(map[string]interface{})["id"].(string)
	settlePath := "/api/v1/receivables/" + recID + "/settle"

	// 5a. Settling a USD receivable into the MYR account without account_amount is
	// refused. The suggestion is 800 x 4.50 = 3600.
	rec = app.request("POST", settlePath, fmt.Sprintf(`{"account_id":%q,"amount":800}`, myrCash), token)
	// (The settlement is dated by the request, so its rate is the latest one at that time.)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("settle without account_amount: %d %s", rec.Code, rec.Body.String())
	}
	apiErr := parseJSON(t, rec)["error"].(map[string]interface{})
	details, _ := apiErr["details"].(map[string]interface{})
	if apiErr["code"] != "FX_AMOUNT_REQUIRED" || details["field"] != "account_amount" || num(details, "suggested_amount") != 3600 {
		t.Errorf("settle error = %v", apiErr)
	}
	if got := num(getJSON("/api/v1/accounts/" + myrCash)["account"].(map[string]interface{}), "balance"); got != 997000 {
		t.Errorf("refused settle moved money: MYR balance %.0f, want 997000", got)
	}

	// 5b. Settle with account_amount = RM 36.00 (3600 cents). The receivable is in
	// USD (800 cents); the MYR account is credited 3600 cents.
	res := createOK(settlePath, fmt.Sprintf(`{"account_id":%q,"amount":800,"account_amount":3600}`, myrCash))
	if r := res["receivable"].(map[string]interface{}); r["status"] != "settled" || num(r, "outstanding") != 0 {
		t.Errorf("settled receivable = %v", r)
	}
	settlementID := res["settlement"].(map[string]interface{})["id"].(string)

	// Balances after the settlement, read through GET /accounts below:
	//   MYR: 1000000 - 3000 + 3600 = 1000600 (RM 10,006.00)
	//   USD: 100000 - 1000 - 2000 - 800 = 96200 (US$962.00)
	accounts := func() map[string]map[string]interface{} {
		t.Helper()
		page := getJSON("/api/v1/accounts")
		out := map[string]map[string]interface{}{}
		for _, row := range page["data"].([]interface{}) {
			a := row.(map[string]interface{})
			out[a["id"].(string)] = a
		}
		return out
	}

	// 6. Monthly summary for the current month.
	// Expenses: US$10.00 x 4.00 (D1) + US$20.00 x 5.00 (D2) + RM 30.00 = 40 + 100 + 30 = RM 170.00 = 17000.
	// The claim's personal share is 0, so it adds nothing.
	// Income: 0. The repayment is a reimbursement, and reimbursements are not
	// income in the monthly summary (see TestSharedExpenseFlow_FullLifecycle).
	monthly := getJSON("/api/v1/transactions/monthly-summary?months=1")["data"].([]interface{})[0].(map[string]interface{})
	if num(monthly, "expenses") != 17000 || num(monthly, "income") != 0 || monthly["currency"] != "MYR" || monthly["fx_missing"] == true {
		t.Errorf("monthly summary = %v, want expenses 17000 income 0 currency MYR", monthly)
	}

	// 7. Spending by category, in MYR. The category total is the same 17000: the
	// claim's personal share is 0, so it does not count as spending.
	spend := getJSON("/api/v1/transactions/spending-by-category?" + monthQuery())
	if num(spend, "total_spent") != 17000 || spend["currency"] != "MYR" || spend["fx_missing"] == true {
		t.Errorf("spending = %v, want total 17000 MYR", spend)
	}
	var travelTotal float64 = -1
	for _, item := range spend["items"].([]interface{}) {
		row := item.(map[string]interface{})
		if id, ok := row["category_id"].(string); ok && id == cat {
			travelTotal = num(row, "total")
		}
	}
	if travelTotal != 17000 {
		t.Errorf("Travel category total = %.0f, want 17000", travelTotal)
	}

	// 8. Budget progress. A budget covers its category across every account, so the
	// RM 30.00 MYR lunch counts too. The USD budget is spent in USD: the USD rows
	// are not converted (US$10.00 + US$20.00 = 3000), and the MYR row is converted
	// at its day's rate (D1, 4.00: 3000 / 4.00 = 750). Total 3750, remaining
	// 5000 - 3750 = 1250, 75%.
	usdProg := getJSON("/api/v1/budgets/" + usdBudget + "/progress")["progress"].(map[string]interface{})
	if num(usdProg, "spent") != 3750 || num(usdProg, "remaining") != 1250 || !approx(num(usdProg, "percentage"), 75) ||
		usdProg["currency"] != "USD" || usdProg["fx_missing"] == true {
		t.Errorf("USD budget progress = %v, want spent 3750 remaining 1250 75%% USD", usdProg)
	}
	// The MYR budget converts each USD row at its own day's rate:
	// 1000 x 4.00 + 2000 x 5.00 + 3000 = 4000 + 10000 + 3000 = 17000, remaining 50000 - 17000 = 33000, 34%.
	myrProg := getJSON("/api/v1/budgets/" + myrBudget + "/progress")["progress"].(map[string]interface{})
	if num(myrProg, "spent") != 17000 || num(myrProg, "remaining") != 33000 || !approx(num(myrProg, "percentage"), 34) ||
		myrProg["currency"] != "MYR" || myrProg["fx_missing"] == true {
		t.Errorf("MYR budget progress = %v, want spent 17000 remaining 33000 34%% MYR", myrProg)
	}

	// 9. Accounts. The USD wallet's balance_base is 96200 x 4.50 = 432900 (RM 4,329.00).
	// The MYR account is already in MYR, so balance_base equals balance.
	accts := accounts()
	usdAcct := accts[usdCash]
	if num(usdAcct, "balance") != 96200 || num(usdAcct, "balance_base") != 432900 {
		t.Errorf("USD account balance %v balance_base %v, want 96200 / 432900", usdAcct["balance"], usdAcct["balance_base"])
	}
	myrAcct := accts[myrCash]
	if num(myrAcct, "balance") != 1000600 || num(myrAcct, "balance_base") != num(myrAcct, "balance") {
		t.Errorf("MYR account balance %v balance_base %v, want 1000600 / equal", myrAcct["balance"], myrAcct["balance_base"])
	}

	// 10. Snapshot. Net worth is in MYR: MYR cash 1000600 + USD cash 96200 x 4.50 (432900) = 1433500.
	// No investments or debt, and no missing rates.
	rec = app.pipelineRequest("POST", "/api/v1/pipeline/snapshots", fmt.Sprintf(`{"recorded_at":%q}`, time.Now().UTC().Format(time.RFC3339)))
	if rec.Code != http.StatusOK {
		t.Fatalf("compute snapshots: %d %s", rec.Code, rec.Body.String())
	}
	fromDate := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	toDate := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	snaps := getJSON(fmt.Sprintf("/api/v1/investments/snapshots?from_date=%s&to_date=%s", fromDate, toDate))
	if num(snaps, "total_items") != 1 {
		t.Fatalf("snapshots = %v", snaps)
	}
	snap := snaps["data"].([]interface{})[0].(map[string]interface{})
	if num(snap, "total_net_worth") != 1433500 || num(snap, "cash_balance") != 1433500 || snap["fx_missing"] != false {
		t.Errorf("snapshot = %v, want total_net_worth 1433500 cash 1433500 fx_missing false", snap)
	}

	// 11. Receivables summary: nothing is outstanding after the settle.
	// Expected: total_outstanding 0.
	summary := getJSON("/api/v1/receivables/summary")
	if num(summary, "total_outstanding") != 0 || summary["fx_missing"] == true {
		t.Errorf("receivables summary = %v, want total_outstanding 0", summary)
	}

	// 12. Deleting the settlement reverses the MYR credit exactly: back to 997000
	// (RM 9,970.00, the pre-settle value). The receivable reopens at 800.
	reopened := app.request("DELETE", "/api/v1/receivables/settlements/"+settlementID, "", token)
	if reopened.Code != http.StatusOK {
		t.Fatalf("delete settlement: %d %s", reopened.Code, reopened.Body.String())
	}
	if r := parseJSON(t, reopened)["receivable"].(map[string]interface{}); r["status"] != "pending" || num(r, "outstanding") != 800 {
		t.Errorf("reopened receivable = %v", r)
	}
	if got := num(accounts()[myrCash], "balance"); got != 997000 {
		t.Errorf("MYR balance after deleting the settlement = %.0f, want 997000", got)
	}
}
