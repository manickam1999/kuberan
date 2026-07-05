package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerSnapshotTools() {
	s.mcp.AddTool(
		mcp.NewTool("get_net_worth_history",
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
	fromStr, _ := args["from_date"].(string)
	toStr, _ := args["to_date"].(string)

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return mcp.NewToolResultError("invalid from_date format, expected YYYY-MM-DD"), nil
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return mcp.NewToolResultError("invalid to_date format, expected YYYY-MM-DD"), nil
	}

	groupBy := "month"
	if v, _ := args["group_by"].(string); v == "day" || v == "week" || v == "month" {
		groupBy = v
	}

	snapshots, err := s.services.Snapshots.GetGroupedSnapshots(userID, from, to, groupBy)
	if err != nil {
		return mcp.NewToolResultError("failed to get net worth history"), nil
	}

	if len(snapshots) == 0 {
		return mcp.NewToolResultText("No net worth data for the requested period."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Net Worth History (%s to %s, grouped by %s):\n\n", fromStr, toStr, groupBy))
	sb.WriteString(fmt.Sprintf("  %-12s %14s %14s %14s %14s\n",
		"Date", "Net Worth", "Cash", "Investments", "Debt"))
	sb.WriteString(fmt.Sprintf("  %-12s %14s %14s %14s %14s\n",
		"----", "---------", "----", "-----------", "----"))

	for _, snap := range snapshots {
		sb.WriteString(fmt.Sprintf("  %-12s %14s %14s %14s %14s\n",
			snap.RecordedAt.Format("2006-01-02"),
			formatCents(snap.TotalNetWorth),
			formatCents(snap.CashBalance),
			formatCents(snap.InvestmentValue),
			formatCents(snap.DebtBalance),
		))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
