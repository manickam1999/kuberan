package mcp

import (
	"testing"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// TestE2EActivityFeedSummarizesMCPWrites checks that the audit rows the tools
// write are rich enough for Settings > Connected apps to say what changed.
func TestE2EActivityFeedSummarizesMCPWrites(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	s := e.as("tok")
	var alice models.Receivable
	e.db.Where("counterparty_name = ?", "Alice").First(&alice)
	var budget models.Budget
	e.db.Where("name = ?", "Groceries budget").First(&budget)

	s.call("create_transaction", map[string]any{
		"account_id": e.acc["wise"].ID, "type": "expense", "amount": 19.99, "date": dateStr(0), "description": "Cinema",
	}).ok(t)
	s.call("create_transfer", map[string]any{
		"from_account_id": e.acc["maybank"].ID, "to_account_id": e.acc["visa"].ID, "amount": 200, "date": dateStr(0), "description": "Pay card",
	}).ok(t)
	s.call("update_transaction", map[string]any{"transaction_id": e.txnByDescription("Cinema").ID, "description": "Cinema night"}).ok(t)
	s.call("update_budget", map[string]any{"budget_id": budget.ID, "name": "Food"}).ok(t)
	s.call("create_category", map[string]any{"name": "Gifts", "type": "expense"}).ok(t)
	s.call("record_repayment", map[string]any{"receivable_id": alice.ID, "account_id": e.acc["maybank"].ID, "amount": 20, "date": dateStr(0)}).ok(t)
	s.call("write_off_receivable", map[string]any{"receivable_id": alice.ID, "amount": 10}).ok(t)

	page, err := services.NewAppActivityService(e.db).ListByClient(e.user.ID, e2eClientID, pagination.PageRequest{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range page.Data {
		got = append(got, item.Summary)
	}
	for _, want := range []string{
		"Added transaction · Cinema · USD 19.99",
		"Added transfer · Pay card · MYR 200.00",
		"Edited transaction · Cinema night",
		"Edited budget · Food",
		"Created category · Gifts",
		"Recorded repayment · MYR 20.00",
		"Wrote off receivable · MYR 10.00",
	} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("activity feed lacks %q; got %q", want, got)
		}
	}
	if len(got) != 7 {
		t.Errorf("%d activity items, want 7: %q", len(got), got)
	}
}
