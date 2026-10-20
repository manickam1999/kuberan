package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// sharedFlow is a small helper around one authenticated user for the
// receivables lifecycle tests.
type sharedFlow struct {
	t     *testing.T
	app   *testApp
	token string
}

func (f *sharedFlow) call(method, path, body string, wantStatus int) map[string]interface{} {
	f.t.Helper()
	rec := f.app.request(method, path, body, f.token)
	if rec.Code != wantStatus {
		f.t.Fatalf("%s %s: expected %d, got %d: %s", method, path, wantStatus, rec.Code, rec.Body.String())
	}
	return parseJSON(f.t, rec)
}

func (f *sharedFlow) callErr(method, path, body string, wantStatus int, wantCode string) {
	f.t.Helper()
	body2 := f.call(method, path, body, wantStatus)
	errObj, ok := body2["error"].(map[string]interface{})
	if !ok || errObj["code"] != wantCode {
		f.t.Fatalf("%s %s: expected error code %s, got %v", method, path, wantCode, body2)
	}
}

func (f *sharedFlow) account(name string, balance int64) string {
	f.t.Helper()
	res := f.call("POST", "/api/v1/accounts/cash", fmt.Sprintf(`{"name":%q,"initial_balance":%d}`, name, balance), http.StatusCreated)
	return res["account"].(map[string]interface{})["id"].(string)
}

func (f *sharedFlow) balance(accountID string) float64 {
	f.t.Helper()
	res := f.call("GET", "/api/v1/accounts/"+accountID, "", http.StatusOK)
	return res["account"].(map[string]interface{})["balance"].(float64)
}

func (f *sharedFlow) wantBalance(accountID string, want float64) {
	f.t.Helper()
	if got := f.balance(accountID); got != want {
		f.t.Errorf("balance = %.0f, want %.0f", got, want)
	}
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

func monthQuery() string {
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)
	return fmt.Sprintf("from_date=%s&to_date=%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
}

func num(m map[string]interface{}, key string) float64 {
	v, ok := m[key].(float64)
	if !ok {
		panic(fmt.Sprintf("field %q is not a number in %v", key, m))
	}
	return v
}

// TestSharedExpenseFlow_FullLifecycle drives a $60 three-way split through
// repayment, reversal, write-off, edit and delete over HTTP.
func TestSharedExpenseFlow_FullLifecycle(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "shared@test.com", "password123")
	f := &sharedFlow{t: t, app: app, token: token}

	acct := f.account("Wallet", 100000)
	cat := f.call("POST", "/api/v1/categories", `{"name":"Dining","type":"expense"}`, http.StatusCreated)["category"].(map[string]interface{})["id"].(string)
	budget := f.call("POST", "/api/v1/budgets", fmt.Sprintf(`{"category_id":%q,"name":"Dining","amount":50000,"period":"monthly"}`, cat), http.StatusCreated)["budget"].(map[string]interface{})["id"].(string)

	// 1. Create a $60 expense split with two friends.
	created := f.call("POST", "/api/v1/transactions/shared", fmt.Sprintf(
		`{"account_id":%q,"category_id":%q,"amount":6000,"description":"Dinner","date":%q,"kind":"split","mode":"equal","counterparties":[{"name":"Alex"},{"name":"Sam"}]}`,
		acct, cat, today()), http.StatusCreated)["transaction"].(map[string]interface{})
	txnID := created["id"].(string)
	if num(created, "amount") != 6000 || num(created, "personal_amount") != 2000 {
		t.Fatalf("created = %v", created)
	}
	recs := created["receivables"].([]interface{})
	if len(recs) != 2 {
		t.Fatalf("receivables = %v", recs)
	}
	alex := recs[0].(map[string]interface{})
	sam := recs[1].(map[string]interface{})
	if alex["counterparty_name"] != "Alex" || num(alex, "amount") != 2000 || num(alex, "outstanding") != 2000 || alex["status"] != "pending" || alex["kind"] != "split" {
		t.Errorf("alex = %v", alex)
	}
	alexID, samID := alex["id"].(string), sam["id"].(string)

	// 2. The account is down the full $60; reports count only $20.
	f.wantBalance(acct, 94000)
	spend := f.call("GET", "/api/v1/transactions/spending-by-category?"+monthQuery(), "", http.StatusOK)
	if num(spend, "total_spent") != 2000 {
		t.Errorf("spending = %v, want 2000", spend)
	}
	progress := f.call("GET", "/api/v1/budgets/"+budget+"/progress", "", http.StatusOK)
	if num(progress["progress"].(map[string]interface{}), "spent") != 2000 {
		t.Errorf("budget progress = %v", progress)
	}
	top := f.call("GET", "/api/v1/transactions/top-expenses?"+monthQuery(), "", http.StatusOK)["items"].([]interface{})
	if len(top) != 1 || num(top[0].(map[string]interface{}), "amount") != 6000 || num(top[0].(map[string]interface{}), "personal_amount") != 2000 {
		t.Errorf("top expenses = %v", top)
	}

	// 3. The transaction reads include the receivables.
	detail := f.call("GET", "/api/v1/transactions/"+txnID, "", http.StatusOK)["transaction"].(map[string]interface{})
	if len(detail["receivables"].([]interface{})) != 2 {
		t.Errorf("detail receivables = %v", detail["receivables"])
	}
	list := f.call("GET", "/api/v1/transactions?type=expense", "", http.StatusOK)["data"].([]interface{})
	var listed map[string]interface{}
	for _, row := range list {
		if row.(map[string]interface{})["id"] == txnID {
			listed = row.(map[string]interface{})
		}
	}
	if listed == nil || num(listed, "personal_amount") != 2000 || len(listed["receivables"].([]interface{})) != 2 {
		t.Errorf("listed expense = %v", listed)
	}

	// 4. Summary, list and autocomplete (static routes next to /:id).
	summary := f.call("GET", "/api/v1/receivables/summary", "", http.StatusOK)
	if num(summary, "total_outstanding") != 4000 || len(summary["by_counterparty"].([]interface{})) != 2 || len(summary["by_kind"].([]interface{})) != 1 {
		t.Errorf("summary = %v", summary)
	}
	open := f.call("GET", "/api/v1/receivables?status=open&kind=split", "", http.StatusOK)
	if num(open, "total_items") != 2 {
		t.Errorf("open list = %v", open)
	}
	first := open["data"].([]interface{})[0].(map[string]interface{})
	if first["transaction"].(map[string]interface{})["id"] != txnID {
		t.Errorf("list item expense = %v", first["transaction"])
	}
	cps := f.call("GET", "/api/v1/receivables/counterparties?q=ale", "", http.StatusOK)["counterparties"].([]interface{})
	if len(cps) != 1 || cps[0].(map[string]interface{})["key"] != "alex" {
		t.Errorf("counterparties = %v", cps)
	}

	// 5. Alex repays $20: the account is credited, income stays 0.
	settled := f.call("POST", "/api/v1/receivables/"+alexID+"/settle",
		fmt.Sprintf(`{"account_id":%q,"amount":2000,"date":%q}`, acct, today()), http.StatusCreated)
	if r := settled["receivable"].(map[string]interface{}); r["status"] != "settled" || num(r, "outstanding") != 0 || num(r, "paid_amount") != 2000 {
		t.Errorf("settled receivable = %v", r)
	}
	settlement := settled["settlement"].(map[string]interface{})
	settlementID := settlement["id"].(string)
	if settlement["account_id"] != acct || num(settlement, "amount") != 2000 || settlement["transaction_id"] == "" {
		t.Errorf("settlement = %v", settlement)
	}
	f.wantBalance(acct, 96000)
	monthly := f.call("GET", "/api/v1/transactions/monthly-summary?months=1", "", http.StatusOK)["data"].([]interface{})[0].(map[string]interface{})
	// The $1000 "Initial balance" income is excluded by the report; the repayment must not appear.
	if num(monthly, "income") != 0 || num(monthly, "expenses") != 2000 {
		t.Errorf("monthly = %v, want income 0 expenses 2000", monthly)
	}
	repayments := f.call("GET", "/api/v1/transactions?type=reimbursement", "", http.StatusOK)
	if num(repayments, "total_items") != 1 {
		t.Fatalf("reimbursements = %v", repayments)
	}
	reimbID := repayments["data"].([]interface{})[0].(map[string]interface{})["id"].(string)
	if r := repayments["data"].([]interface{})[0].(map[string]interface{}); r["description"] != "Repayment from Alex: Dinner" {
		t.Errorf("reimbursement description = %v", r["description"])
	}

	// 6. A reimbursement can't be edited or deleted directly; the expense can't be
	//    deleted while a repayment exists.
	f.callErr("PUT", "/api/v1/transactions/"+reimbID, `{"amount":1}`, http.StatusBadRequest, "TRANSACTION_NOT_EDITABLE")
	f.callErr("DELETE", "/api/v1/transactions/"+reimbID, "", http.StatusBadRequest, "TRANSACTION_NOT_EDITABLE")
	f.callErr("DELETE", "/api/v1/transactions/"+txnID, "", http.StatusConflict, "RECEIVABLE_HAS_SETTLEMENTS")
	f.callErr("POST", "/api/v1/transactions", fmt.Sprintf(`{"account_id":%q,"type":"reimbursement","amount":5}`, acct), http.StatusBadRequest, "INVALID_INPUT")

	// 7. Overpaying, a wrong account type and unknown ids are rejected.
	f.callErr("POST", "/api/v1/receivables/"+samID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":2001}`, acct), http.StatusBadRequest, "SETTLEMENT_EXCEEDS_OUTSTANDING")
	inv := f.call("POST", "/api/v1/accounts/investment", `{"name":"Brokerage"}`, http.StatusCreated)["account"].(map[string]interface{})["id"].(string)
	f.callErr("POST", "/api/v1/receivables/"+samID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":100}`, inv), http.StatusBadRequest, "INVALID_SETTLEMENT_ACCOUNT")
	f.callErr("GET", "/api/v1/receivables/00000000-0000-0000-0000-000000000000", "", http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	f.callErr("DELETE", "/api/v1/receivables/settlements/00000000-0000-0000-0000-000000000000", "", http.StatusNotFound, "SETTLEMENT_NOT_FOUND")
	f.wantBalance(acct, 96000)

	// 8. Sam pays $5 into a second account, then the rest is written off and undone.
	other := f.account("Savings", 0)
	part := f.call("POST", "/api/v1/receivables/"+samID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":500}`, other), http.StatusCreated)
	if r := part["receivable"].(map[string]interface{}); r["status"] != "partially_paid" || num(r, "outstanding") != 1500 {
		t.Errorf("sam after part payment = %v", r)
	}
	f.wantBalance(other, 500)
	wo := f.call("POST", "/api/v1/receivables/"+samID+"/write-off", ``, http.StatusOK)["receivable"].(map[string]interface{})
	if wo["status"] != "written_off" || num(wo, "written_off_amount") != 1500 || num(wo, "outstanding") != 0 {
		t.Errorf("written off sam = %v", wo)
	}
	// $15 more is now spending, in the expense's month.
	spend = f.call("GET", "/api/v1/transactions/spending-by-category?"+monthQuery(), "", http.StatusOK)
	if num(spend, "total_spent") != 3500 {
		t.Errorf("spending after write-off = %v, want 3500", spend)
	}
	undone := f.call("DELETE", "/api/v1/receivables/"+samID+"/write-off", "", http.StatusOK)["receivable"].(map[string]interface{})
	if undone["status"] != "partially_paid" || num(undone, "written_off_amount") != 0 || num(undone, "outstanding") != 1500 {
		t.Errorf("undone sam = %v", undone)
	}
	f.callErr("DELETE", "/api/v1/receivables/"+samID+"/write-off", "", http.StatusBadRequest, "INVALID_INPUT")
	f.callErr("POST", "/api/v1/receivables/"+samID+"/write-off", `{"amount":1501}`, http.StatusBadRequest, "WRITE_OFF_EXCEEDS_OUTSTANDING")

	// 9. Receivable detail carries its settlements.
	detailRec := f.call("GET", "/api/v1/receivables/"+samID, "", http.StatusOK)["receivable"].(map[string]interface{})
	if sets := detailRec["settlements"].([]interface{}); len(sets) != 1 || num(sets[0].(map[string]interface{}), "amount") != 500 {
		t.Errorf("sam settlements = %v", detailRec["settlements"])
	}

	// 10. Deleting Alex's repayment debits the account exactly once and reopens it.
	reopened := f.call("DELETE", "/api/v1/receivables/settlements/"+settlementID, "", http.StatusOK)["receivable"].(map[string]interface{})
	if reopened["status"] != "pending" || num(reopened, "outstanding") != 2000 {
		t.Errorf("reopened alex = %v", reopened)
	}
	f.wantBalance(acct, 94000)
	f.callErr("DELETE", "/api/v1/receivables/settlements/"+settlementID, "", http.StatusNotFound, "SETTLEMENT_NOT_FOUND")
	f.wantBalance(acct, 94000)

	// 11. Rename Alex, then edit the shares by diff.
	renamed := f.call("POST", "/api/v1/receivables/counterparties/rename", `{"from_key":"alex","to_name":"Alexander  Tan"}`, http.StatusOK)
	if num(renamed, "updated") != 1 {
		t.Errorf("rename = %v", renamed)
	}
	f.callErr("PUT", "/api/v1/transactions/"+txnID+"/receivables",
		`{"receivables":[{"id":"`+alexID+`","name":"Alexander Tan","amount":2000}]}`, http.StatusConflict, "RECEIVABLE_HAS_PAYMENTS") // Sam has a payment
	edited := f.call("PUT", "/api/v1/transactions/"+txnID+"/receivables",
		fmt.Sprintf(`{"kind":"claim","receivables":[{"id":%q,"name":"Alexander Tan","amount":2500},{"id":%q,"name":"Sam","amount":2000},{"name":"Kim","amount":500}]}`, alexID, samID),
		http.StatusOK)["transaction"].(map[string]interface{})
	if num(edited, "personal_amount") != 1000 || len(edited["receivables"].([]interface{})) != 3 {
		t.Errorf("edited = %v", edited)
	}
	if k := edited["receivables"].([]interface{})[2].(map[string]interface{})["kind"]; k != "claim" {
		t.Errorf("kind = %v", k)
	}
	f.callErr("PUT", "/api/v1/transactions/"+txnID+"/receivables",
		fmt.Sprintf(`{"receivables":[{"id":%q,"name":"Sam","amount":9000}]}`, samID), http.StatusBadRequest, "INVALID_SHARES")

	// 12. Editing the expense itself: amount below Σ shares and type changes are rejected.
	f.callErr("PUT", "/api/v1/transactions/"+txnID, `{"amount":4999}`, http.StatusBadRequest, "INVALID_SHARES")
	f.callErr("PUT", "/api/v1/transactions/"+txnID, `{"type":"income"}`, http.StatusBadRequest, "INVALID_TYPE_CHANGE")
	grown := f.call("PUT", "/api/v1/transactions/"+txnID, `{"amount":8000}`, http.StatusOK)["transaction"].(map[string]interface{})
	if num(grown, "personal_amount") != 3000 {
		t.Errorf("grown = %v", grown)
	}
	f.wantBalance(acct, 92000)

	// 13. Delete Sam's repayment, then the expense (cascade) restores the balance.
	samDetail := f.call("GET", "/api/v1/receivables/"+samID, "", http.StatusOK)["receivable"].(map[string]interface{})
	samSettlement := samDetail["settlements"].([]interface{})[0].(map[string]interface{})["id"].(string)
	f.call("DELETE", "/api/v1/receivables/settlements/"+samSettlement, "", http.StatusOK)
	f.wantBalance(other, 0)
	f.call("DELETE", "/api/v1/transactions/"+txnID, "", http.StatusOK)
	f.wantBalance(acct, 100000)
	f.callErr("GET", "/api/v1/receivables/"+alexID, "", http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	end := f.call("GET", "/api/v1/receivables/summary", "", http.StatusOK)
	if num(end, "total_outstanding") != 0 {
		t.Errorf("final summary = %v", end)
	}
	f.callErr("GET", "/api/v1/transactions/"+txnID, "", http.StatusNotFound, "TRANSACTION_NOT_FOUND")
}

// TestSharedExpenseFlow_FullModeLoanAndWriteOffOnSettle covers the zero-share
// kinds: a loan has no category and does not count as spending, and a deposit
// returned in part can be closed by writing off the remainder in one call.
func TestSharedExpenseFlow_FullModeLoanAndWriteOffOnSettle(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "loan@test.com", "password123")
	f := &sharedFlow{t: t, app: app, token: token}
	acct := f.account("Wallet", 500000)

	loan := f.call("POST", "/api/v1/transactions/shared", fmt.Sprintf(
		`{"account_id":%q,"amount":50000,"description":"Lend Alex","kind":"loan","mode":"full","counterparties":[{"name":"Alex"}]}`, acct),
		http.StatusCreated)["transaction"].(map[string]interface{})
	if num(loan, "personal_amount") != 0 {
		t.Errorf("loan = %v", loan)
	}
	if _, has := loan["category_id"]; has {
		t.Errorf("a loan must have no category: %v", loan["category_id"])
	}
	f.wantBalance(acct, 450000)
	spend := f.call("GET", "/api/v1/transactions/spending-by-category?"+monthQuery(), "", http.StatusOK)
	if num(spend, "total_spent") != 0 {
		t.Errorf("a loan is not spending: %v", spend)
	}
	top := f.call("GET", "/api/v1/transactions/top-expenses?"+monthQuery(), "", http.StatusOK)["items"].([]interface{})
	if len(top) != 0 {
		t.Errorf("top expenses should skip a $0 share: %v", top)
	}

	recID := loan["receivables"].([]interface{})[0].(map[string]interface{})["id"].(string)
	res := f.call("POST", "/api/v1/receivables/"+recID+"/settle",
		fmt.Sprintf(`{"account_id":%q,"amount":42500,"write_off_remainder":true}`, acct), http.StatusCreated)
	if r := res["receivable"].(map[string]interface{}); r["status"] != "written_off" || num(r, "written_off_amount") != 7500 || num(r, "outstanding") != 0 {
		t.Errorf("receivable = %v", r)
	}
	f.wantBalance(acct, 492500)
	spend = f.call("GET", "/api/v1/transactions/spending-by-category?"+monthQuery(), "", http.StatusOK)
	if num(spend, "total_spent") != 7500 {
		t.Errorf("forgiven part counts as spending: %v", spend)
	}
}

func TestSharedExpenseFlow_Validation(t *testing.T) {
	app := setupApp(t)
	token, _, _ := app.registerUser(t, "valid@test.com", "password123")
	f := &sharedFlow{t: t, app: app, token: token}
	acct := f.account("Wallet", 100000)
	post := func(body string, status int, code string) {
		t.Helper()
		f.callErr("POST", "/api/v1/transactions/shared", body, status, code)
	}

	post(fmt.Sprintf(`{"account_id":%q,"amount":6000,"kind":"gift","mode":"equal","counterparties":[{"name":"A"}]}`, acct), http.StatusBadRequest, "INVALID_RECEIVABLE_KIND")
	post(fmt.Sprintf(`{"account_id":%q,"amount":6000,"kind":"split","mode":"equal","counterparties":[]}`, acct), http.StatusBadRequest, "INVALID_SHARES")
	post(fmt.Sprintf(`{"account_id":%q,"amount":6000,"kind":"split","mode":"equal","counterparties":[{"name":"A"},{"name":" a "}]}`, acct), http.StatusBadRequest, "INVALID_SHARES")
	post(fmt.Sprintf(`{"account_id":%q,"amount":6000,"kind":"split","mode":"custom","counterparties":[{"name":"A","amount":7000}]}`, acct), http.StatusBadRequest, "INVALID_SHARES")
	post(fmt.Sprintf(`{"account_id":%q,"amount":6000,"kind":"split","mode":"nope","counterparties":[{"name":"A"}]}`, acct), http.StatusBadRequest, "INVALID_INPUT")
	post(`{"account_id":"00000000-0000-0000-0000-000000000000","amount":6000,"kind":"split","mode":"equal","counterparties":[{"name":"A"}]}`, http.StatusNotFound, "ACCOUNT_NOT_FOUND")
	f.wantBalance(acct, 100000)
}

// TestSharedExpenseFlow_UserIsolationAndAuth checks that receivables of one user
// are invisible and immutable to another, and that every route needs a token.
func TestSharedExpenseFlow_UserIsolationAndAuth(t *testing.T) {
	app := setupApp(t)
	tokenA, _, _ := app.registerUser(t, "owner@test.com", "password123")
	tokenB, _, _ := app.registerUser(t, "intruder@test.com", "password123")
	a := &sharedFlow{t: t, app: app, token: tokenA}
	b := &sharedFlow{t: t, app: app, token: tokenB}

	acctA := a.account("A wallet", 100000)
	acctB := b.account("B wallet", 100000)
	txn := a.call("POST", "/api/v1/transactions/shared", fmt.Sprintf(
		`{"account_id":%q,"amount":6000,"kind":"split","mode":"equal","counterparties":[{"name":"Alex"},{"name":"Sam"}]}`, acctA), http.StatusCreated)["transaction"].(map[string]interface{})
	txnID := txn["id"].(string)
	recID := txn["receivables"].([]interface{})[0].(map[string]interface{})["id"].(string)
	settle := a.call("POST", "/api/v1/receivables/"+recID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":500}`, acctA), http.StatusCreated)
	settlementID := settle["settlement"].(map[string]interface{})["id"].(string)

	b.callErr("GET", "/api/v1/receivables/"+recID, "", http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	b.callErr("POST", "/api/v1/receivables/"+recID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":100}`, acctB), http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	b.callErr("POST", "/api/v1/receivables/"+recID+"/write-off", `{}`, http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	b.callErr("DELETE", "/api/v1/receivables/"+recID+"/write-off", "", http.StatusNotFound, "RECEIVABLE_NOT_FOUND")
	b.callErr("DELETE", "/api/v1/receivables/settlements/"+settlementID, "", http.StatusNotFound, "SETTLEMENT_NOT_FOUND")
	b.callErr("PUT", "/api/v1/transactions/"+txnID+"/receivables", `{"receivables":[]}`, http.StatusNotFound, "TRANSACTION_NOT_FOUND")
	b.callErr("POST", "/api/v1/transactions/shared", fmt.Sprintf(
		`{"account_id":%q,"amount":100,"kind":"loan","mode":"full","counterparties":[{"name":"X"}]}`, acctA), http.StatusNotFound, "ACCOUNT_NOT_FOUND")
	a.callErr("POST", "/api/v1/receivables/"+recID+"/settle", fmt.Sprintf(`{"account_id":%q,"amount":100}`, acctB), http.StatusNotFound, "ACCOUNT_NOT_FOUND")

	if list := b.call("GET", "/api/v1/receivables", "", http.StatusOK); num(list, "total_items") != 0 {
		t.Errorf("B sees A's receivables: %v", list)
	}
	if sum := b.call("GET", "/api/v1/receivables/summary", "", http.StatusOK); num(sum, "total_outstanding") != 0 {
		t.Errorf("B sees A's summary: %v", sum)
	}
	if cps := b.call("GET", "/api/v1/receivables/counterparties", "", http.StatusOK)["counterparties"].([]interface{}); len(cps) != 0 {
		t.Errorf("B sees A's counterparties: %v", cps)
	}
	if r := b.call("POST", "/api/v1/receivables/counterparties/rename", `{"from_key":"alex","to_name":"Mallory"}`, http.StatusOK); num(r, "updated") != 0 {
		t.Errorf("B renamed A's counterparties: %v", r)
	}
	a.wantBalance(acctA, 94500)
	b.wantBalance(acctB, 100000)
	if got := a.call("GET", "/api/v1/receivables/"+recID, "", http.StatusOK)["receivable"].(map[string]interface{}); got["counterparty_name"] != "Alex" {
		t.Errorf("A's receivable changed: %v", got)
	}

	routes := []struct{ method, path string }{
		{"POST", "/api/v1/transactions/shared"},
		{"PUT", "/api/v1/transactions/" + txnID + "/receivables"},
		{"GET", "/api/v1/receivables"},
		{"GET", "/api/v1/receivables/summary"},
		{"GET", "/api/v1/receivables/counterparties"},
		{"POST", "/api/v1/receivables/counterparties/rename"},
		{"GET", "/api/v1/receivables/" + recID},
		{"POST", "/api/v1/receivables/" + recID + "/settle"},
		{"DELETE", "/api/v1/receivables/settlements/" + settlementID},
		{"POST", "/api/v1/receivables/" + recID + "/write-off"},
		{"DELETE", "/api/v1/receivables/" + recID + "/write-off"},
	}
	for _, rt := range routes {
		t.Run("401 "+rt.method+" "+rt.path, func(t *testing.T) {
			rec := app.request(rt.method, rt.path, `{}`, "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
