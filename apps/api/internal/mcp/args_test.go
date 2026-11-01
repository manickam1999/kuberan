package mcp

import (
	"context"
	"math"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func TestDateRangeArgs(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		maxDays int
		wantErr string
	}{
		{"one day", "2026-01-01", "2026-01-01", maxDailyRangeDays, ""},
		{"full leap year", "2024-01-01", "2024-12-31", maxDailyRangeDays, ""},
		{"367 days", "2024-01-01", "2025-01-01", maxDailyRangeDays, "too long"},
		{"decades are fine for aggregates", "2016-01-01", "2025-12-31", maxAggregateRangeDays, ""},
		{"reversed", "2026-02-01", "2026-01-01", maxDailyRangeDays, "before from_date"},
		{"bad format", "2026/01/01", "2026-01-02", maxDailyRangeDays, "invalid from_date"},
		{"missing", "", "2026-01-02", maxDailyRangeDays, "invalid from_date"},
		{"year too early", "1969-12-31", "1970-01-01", maxDailyRangeDays, "between 1970 and 2100"},
		{"year too late", "2100-12-31", "2101-01-01", maxDailyRangeDays, "between 1970 and 2100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, res := dateRangeArgs(map[string]interface{}{"from_date": tt.from, "to_date": tt.to}, tt.maxDays)
			if tt.wantErr == "" {
				if res != nil {
					t.Fatalf("unexpected error: %s", resultText(t, res))
				}
				return
			}
			if res == nil || !res.IsError || !strings.Contains(resultText(t, res), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, res)
			}
		})
	}
}

func TestNumericArgBounds(t *testing.T) {
	months := []struct {
		in   interface{}
		want int
	}{
		{nil, 6}, {0.0, 1}, {-5.0, 1}, {3.0, 3}, {24.0, 24}, {1e9, 24}, {math.NaN(), 6}, {math.Inf(1), 24}, {"12", 6},
	}
	for _, m := range months {
		args := map[string]interface{}{}
		if m.in != nil {
			args["months"] = m.in
		}
		if got := monthsArg(args); got != m.want {
			t.Errorf("months %v: got %d, want %d", m.in, got, m.want)
		}
	}

	if got := limitArg(map[string]interface{}{"limit": 1e18}, "limit", 10); got != maxPageSize {
		t.Errorf("limit must be capped at %d, got %d", maxPageSize, got)
	}
	if got := limitArg(map[string]interface{}{"limit": -1.0}, "limit", 10); got != 10 {
		t.Errorf("a negative limit must use the default, got %d", got)
	}

	page := parsePage(map[string]interface{}{"page": 1e18, "page_size": 1e18})
	if page.Page != maxPage || page.PageSize != maxPageSize {
		t.Errorf("page = %+v, want capped at %d/%d", page, maxPage, maxPageSize)
	}

	for _, bad := range []interface{}{1e300, math.Inf(-1), math.NaN(), "10"} {
		if _, res := amountFilterArg(map[string]interface{}{"min_amount": bad}, "min_amount"); res == nil {
			t.Errorf("amount filter %v must be rejected", bad)
		}
	}
	if cents, res := amountFilterArg(map[string]interface{}{"min_amount": 12.5}, "min_amount"); res != nil || *cents != 1250 {
		t.Errorf("12.5 dollars = %v cents (res %v), want 1250", cents, res)
	}
}

// The read tools reject out-of-range arguments before touching the (nil)
// service layer.
func TestReadToolsRejectUnboundedRanges(t *testing.T) {
	s := &Server{}
	ctx := context.WithValue(context.Background(), userIDKey{}, "user-1")
	ctx = context.WithValue(ctx, scopesKey{}, []string{"read:transactions", "read:snapshots"})

	call := func(h func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error), args map[string]any) *mcpgo.CallToolResult {
		t.Helper()
		res, err := h(ctx, callReq(args))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	long := map[string]any{"from_date": "2020-01-01", "to_date": "2026-01-01"}
	wantError(t, call(s.handleGetDailySpending, long), "too long")
	wantError(t, call(s.handleGetDailySummary, long), "too long")
	wantError(t, call(s.handleGetNetWorthHistory, map[string]any{
		"from_date": "2020-01-01", "to_date": "2026-01-01", "group_by": "day",
	}), "too long")
	wantError(t, call(s.handleGetTopExpenses, map[string]any{"from_date": "1900-01-01", "to_date": "2026-01-01"}), "between 1970 and 2100")
	wantError(t, call(s.handleGetSpendingByCategory, map[string]any{"from_date": "2026-01-02", "to_date": "2026-01-01"}), "before from_date")
	wantError(t, call(s.handleListTransactions, map[string]any{"from_date": "not-a-date"}), "invalid from_date")
	wantError(t, call(s.handleListTransactions, map[string]any{"max_amount": 1e300}), "max_amount")
}
