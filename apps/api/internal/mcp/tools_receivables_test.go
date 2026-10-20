package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// mockReceivables stubs ReceivableServicer; unimplemented methods panic via
// the nil embedded interface.
type mockReceivables struct {
	services.ReceivableServicer
	listFn    func(userID string, page pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error)
	summaryFn func(userID string) (*services.ReceivableSummary, error)
}

func (m mockReceivables) ListReceivables(u string, p pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
	return m.listFn(u, p, f)
}

func (m mockReceivables) Summary(u string) (*services.ReceivableSummary, error) {
	return m.summaryFn(u)
}

// mockTransactions stubs TransactionServicer for list_transactions.
type mockTransactions struct {
	services.TransactionServicer
	listFn func(userID string, page pagination.PageRequest, f services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	topFn  func(userID string, from, to time.Time, limit int, categoryID *string) (*services.TopExpenses, error)
}

func (m mockTransactions) GetTopExpenses(u string, from, to time.Time, limit int, c *string) (*services.TopExpenses, error) {
	return m.topFn(u, from, to, limit, c)
}

func (m mockTransactions) GetUserTransactions(u string, p pagination.PageRequest, f services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
	return m.listFn(u, p, f)
}

func authCtx(scopes ...string) context.Context {
	ctx := context.WithValue(context.Background(), userIDKey{}, "user-1")
	return context.WithValue(ctx, scopesKey{}, scopes)
}

func callReq(args map[string]any) mcpgo.CallToolRequest {
	var r mcpgo.CallToolRequest
	r.Params.Arguments = args
	return r
}

func resultText(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty result")
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return tc.Text
}

func TestReceivableToolsEnforceScope(t *testing.T) {
	s := &Server{}
	ctx := authCtx("read:budgets")
	for name, h := range map[string]func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error){
		"list":    s.handleListReceivables,
		"summary": s.handleGetReceivablesSummary,
	} {
		res, err := h(ctx, callReq(nil))
		if err != nil || res == nil || !res.IsError {
			t.Fatalf("%s: expected scope error, got %v %v", name, res, err)
		}
		res, _ = h(context.Background(), callReq(nil))
		if !res.IsError || !strings.Contains(resultText(t, res), "unauthorized") {
			t.Fatalf("%s: expected unauthorized", name)
		}
	}
}

func TestListReceivables(t *testing.T) {
	date := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	var gotFilter services.ReceivableFilter
	var gotPage pagination.PageRequest
	svc := mockReceivables{listFn: func(_ string, p pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
		gotFilter, gotPage = f, p
		resp := pagination.NewPageResponse([]models.Receivable{{
			Base:             models.Base{ID: "r1"},
			CounterpartyName: "Alice Smith",
			Kind:             models.ReceivableKindSplit,
			Amount:           3000,
			PaidAmount:       1000,
			WrittenOffAmount: 500,
			Status:           models.ReceivableStatusPartiallyPaid,
			Transaction:      &models.ReceivableExpense{Description: "Dinner", Date: date, Amount: 9000},
		}}, p.Page, p.PageSize, 1)
		return &resp, nil
	}}
	s := &Server{services: Services{Receivables: svc}}

	res, err := s.handleListReceivables(authCtx("read:transactions"), callReq(map[string]any{
		"status": "open", "kind": "split", "counterparty": "  ALICE   smith ", "page_size": 500.0,
	}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: %v %v", err, res)
	}
	if gotFilter.Status != "open" || gotFilter.Kind != "split" || gotFilter.CounterpartyKey != "  ALICE   smith " /* normalized by the service */ {
		t.Errorf("filter = %+v", gotFilter)
	}
	if gotPage.PageSize != 100 || gotPage.Page != 1 {
		t.Errorf("page = %+v", gotPage)
	}
	out := resultText(t, res)
	for _, want := range []string{
		"Alice Smith", "[split]", "$30.00", "partially_paid",
		"paid $10.00, written off $5.00, outstanding $15.00",
		"Expense: Dinner on 2026-10-05 (total $90.00)", "ID: r1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListReceivablesInvalidInput(t *testing.T) {
	s := &Server{services: Services{Receivables: mockReceivables{}}}
	for _, args := range []map[string]any{{"status": "bogus"}, {"kind": "gift"}} {
		res, err := s.handleListReceivables(authCtx("read:transactions"), callReq(args))
		if err != nil || !res.IsError || !strings.Contains(resultText(t, res), "invalid") {
			t.Errorf("args %v: expected invalid error, got %v %v", args, res, err)
		}
	}
}

func TestListReceivablesEmptyAndError(t *testing.T) {
	empty := mockReceivables{listFn: func(string, pagination.PageRequest, services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
		resp := pagination.NewPageResponse([]models.Receivable{}, 1, 20, 0)
		return &resp, nil
	}}
	s := &Server{services: Services{Receivables: empty}}
	res, _ := s.handleListReceivables(authCtx("read:transactions"), callReq(nil))
	if res.IsError || resultText(t, res) != "No receivables found." {
		t.Errorf("unexpected empty result: %q", resultText(t, res))
	}

	failing := mockReceivables{listFn: func(string, pagination.PageRequest, services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
		return nil, errors.New("db down")
	}}
	s = &Server{services: Services{Receivables: failing}}
	res, _ = s.handleListReceivables(authCtx("read:transactions"), callReq(nil))
	if !res.IsError || strings.Contains(resultText(t, res), "db down") {
		t.Errorf("expected sanitized error, got %q", resultText(t, res))
	}
}

func TestGetReceivablesSummary(t *testing.T) {
	svc := mockReceivables{summaryFn: func(string) (*services.ReceivableSummary, error) {
		return &services.ReceivableSummary{
			TotalOutstanding: 4500,
			ByKind:           []services.ReceivableKindTotal{{Kind: models.ReceivableKindSplit, Outstanding: 4500, OpenCount: 2}},
			ByCounterparty: []services.ReceivableCounterpartyTotal{
				{CounterpartyName: "Alice", Outstanding: 3000, OpenCount: 1, OldestDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
				{CounterpartyName: "Bob", Outstanding: 1500, OpenCount: 1, OldestDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
			},
		}, nil
	}}
	s := &Server{services: Services{Receivables: svc}}
	res, err := s.handleGetReceivablesSummary(authCtx("read:transactions"), callReq(nil))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: %v %v", err, res)
	}
	out := resultText(t, res)
	for _, want := range []string{
		"$45.00 outstanding", "NOT included in net worth", "By counterparty:",
		"Alice", "$30.00", "oldest 2026-09-01", "By kind:", "split", "(2 open)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "Alice") > strings.Index(out, "Bob") {
		t.Errorf("service order not preserved:\n%s", out)
	}
}

func TestGetReceivablesSummaryEmptyAndError(t *testing.T) {
	s := &Server{services: Services{Receivables: mockReceivables{summaryFn: func(string) (*services.ReceivableSummary, error) {
		return &services.ReceivableSummary{}, nil
	}}}}
	res, _ := s.handleGetReceivablesSummary(authCtx("read:transactions"), callReq(nil))
	out := resultText(t, res)
	if res.IsError || !strings.Contains(out, "No open receivables") || !strings.Contains(out, "NOT included in net worth") {
		t.Errorf("unexpected empty output: %q", out)
	}

	s = &Server{services: Services{Receivables: mockReceivables{summaryFn: func(string) (*services.ReceivableSummary, error) {
		return nil, errors.New("boom")
	}}}}
	res, _ = s.handleGetReceivablesSummary(authCtx("read:transactions"), callReq(nil))
	if !res.IsError || strings.Contains(resultText(t, res), "boom") {
		t.Errorf("expected sanitized error")
	}
}

func TestListTransactionsTypeWhitelist(t *testing.T) {
	var gotType *models.TransactionType
	svc := mockTransactions{listFn: func(_ string, p pagination.PageRequest, f services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
		gotType = f.Type
		resp := pagination.NewPageResponse([]models.Transaction{}, p.Page, p.PageSize, 0)
		return &resp, nil
	}}
	s := &Server{services: Services{Transactions: svc}}
	ctx := authCtx("read:transactions")

	for _, typ := range []string{"income", "expense", "transfer", "investment", "reimbursement"} {
		gotType = nil
		res, err := s.handleListTransactions(ctx, callReq(map[string]any{"type": typ}))
		if err != nil || res.IsError {
			t.Fatalf("type %s rejected: %v %v", typ, res, err)
		}
		if gotType == nil || string(*gotType) != typ {
			t.Errorf("type %s not forwarded: %v", typ, gotType)
		}
	}

	res, _ := s.handleListTransactions(ctx, callReq(map[string]any{"type": "bogus"}))
	if !res.IsError || !strings.Contains(resultText(t, res), "income, expense, transfer, investment, reimbursement") {
		t.Errorf("unexpected error text: %q", resultText(t, res))
	}
}

func TestListTransactionsSharedAndReimbursementOutput(t *testing.T) {
	personal := int64(2000)
	date := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	svc := mockTransactions{listFn: func(_ string, p pagination.PageRequest, _ services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
		resp := pagination.NewPageResponse([]models.Transaction{
			{Base: models.Base{ID: "t1"}, Type: models.TransactionTypeExpense, Amount: 6000, PersonalAmount: &personal, Description: "Dinner", Date: date},
			{Base: models.Base{ID: "t2"}, Type: models.TransactionTypeReimbursement, Amount: 2000, Description: "Repayment from Alice", Date: date},
			{Base: models.Base{ID: "t3"}, Type: models.TransactionTypeExpense, Amount: 500, Description: "Coffee", Date: date},
		}, p.Page, p.PageSize, 3)
		return &resp, nil
	}}
	s := &Server{services: Services{Transactions: svc}}
	res, err := s.handleListTransactions(authCtx("read:transactions"), callReq(nil))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: %v %v", err, res)
	}
	out := resultText(t, res)
	if !strings.Contains(out, "Shared expense: your share $20.00 of $60.00") {
		t.Errorf("missing shared note:\n%s", out)
	}
	if !strings.Contains(out, "Repayment received for a shared expense (not income)") {
		t.Errorf("missing repayment label:\n%s", out)
	}
	if strings.Count(out, "Shared expense:") != 1 {
		t.Errorf("unshared expense must not get a note:\n%s", out)
	}
}

func TestGetTopExpensesShowsEffectiveAmount(t *testing.T) {
	personal := int64(2000)
	svc := mockTransactions{topFn: func(string, time.Time, time.Time, int, *string) (*services.TopExpenses, error) {
		return &services.TopExpenses{Items: []services.TopExpenseItem{
			{Description: "Group dinner", Amount: 9000, PersonalAmount: &personal, CategoryName: "Food", AccountName: "Cash", Date: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
			{Description: "Shoes", Amount: 1500, CategoryName: "Shopping", AccountName: "Cash", Date: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)},
		}}, nil
	}}
	s := &Server{services: Services{Transactions: svc}}

	res, err := s.handleGetTopExpenses(authCtx("read:transactions"), callReq(map[string]any{
		"from_date": "2026-10-01", "to_date": "2026-10-31",
	}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: %v %v", err, res)
	}
	text := resultText(t, res)

	var dinnerLine string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Group dinner") {
			dinnerLine = line
		}
	}
	if !strings.Contains(dinnerLine, "$20.00") || strings.Contains(dinnerLine, "$90.00") {
		t.Errorf("shared row must show the effective amount, got line %q", dinnerLine)
	}
	if !strings.Contains(text, "your share $20.00 of $90.00") {
		t.Errorf("missing share note in:\n%s", text)
	}
	if !strings.Contains(text, "$15.00") {
		t.Errorf("unshared row must show its full amount in:\n%s", text)
	}
}
