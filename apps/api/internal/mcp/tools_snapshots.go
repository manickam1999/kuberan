package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerSnapshotTools() {
	s.mcp.AddTool(
		readTool("get_net_worth_history",
			mcp.WithDescription("Get historical net worth snapshots showing total net worth, cash, investments, and debt over time."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
			mcp.WithString("group_by", mcp.Description("Grouping: 'day', 'week', or 'month' (default 'month')")),
		),
		s.handleGetNetWorthHistory,
	)
}

func (s *Server) handleGetNetWorthHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:snapshots"); denied != nil {
		return denied, nil
	}

	args := req.GetArguments()
	groupBy := "month"
	if v, _ := args["group_by"].(string); v == "day" || v == "week" || v == "month" {
		groupBy = v
	}

	// One row per group: daily grouping gets the per-day cap.
	maxDays := maxAggregateRangeDays
	if groupBy == "day" {
		maxDays = maxDailyRangeDays
	}
	from, to, res := dateRangeArgs(args, maxDays)
	if res != nil {
		return res, nil
	}
	fromStr, toStr := from.Format("2006-01-02"), to.Format("2006-01-02")

	snapshots, err := s.services.Snapshots.GetGroupedSnapshots(userID, from, to, groupBy)
	if err != nil {
		return mcp.NewToolResultError("failed to get net worth history"), nil
	}

	if len(snapshots) == 0 {
		return mcp.NewToolResultText("No net worth data for the requested period."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Net Worth History (%s to %s, grouped by %s):\n\n", fromStr, toStr, groupBy)
	fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s\n",
		"Date", "Cur", "Net Worth", "Cash", "Investments", "Debt")
	fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s\n",
		"----", "---", "---------", "----", "-----------", "----")

	changed, missing := false, false
	for i, snap := range snapshots {
		cur := snap.Currency
		fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s\n",
			snap.RecordedAt.Format("2006-01-02"), currencyCode(cur),
			formatMoney(snap.TotalNetWorth, cur),
			formatMoney(snap.CashBalance, cur),
			formatMoney(snap.InvestmentValue, cur),
			formatMoney(snap.DebtBalance, cur))
		if i > 0 && !strings.EqualFold(cur, snapshots[i-1].Currency) {
			changed = true
		}
		missing = missing || snap.FXMissing
	}
	if changed {
		sb.WriteString("Note: the reporting currency changed during this period; each row is in its own currency and rows are not comparable\n")
	}
	if missing {
		sb.WriteString("Note: some points exclude amounts with no exchange rate (fx_missing), so they understate net worth\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
