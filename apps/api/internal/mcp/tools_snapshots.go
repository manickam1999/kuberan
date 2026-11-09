package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
)

func (s *Server) registerSnapshotTools() {
	s.mcp.AddTool(
		readTool("get_net_worth_history",
			mcp.WithDescription("Get historical net worth snapshots in the user's base currency: net worth (cash + fixed deposits + investments - debt, where debt covers credit cards and loans), and each part, over time. Each row is the last snapshot of its day, week or month."),
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

	// The service buckets by day (or hour) only; week and month are cut here.
	snapshots, err := s.services.Snapshots.GetGroupedSnapshots(userID, from, to, "day")
	if err != nil {
		return readError("get net worth history", err), nil
	}
	snapshots = downsampleSnapshots(snapshots, groupBy)

	if len(snapshots) == 0 {
		return mcp.NewToolResultText("No net worth data for the requested period."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Net Worth History (%s to %s, grouped by %s):\n\n", fromStr, toStr, groupBy)
	fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s %18s\n",
		"Date", "Cur", "Net Worth", "Cash", "Fixed deposits", "Investments", "Debt")
	fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s %18s\n",
		"----", "---", "---------", "----", "--------------", "-----------", "----")

	changed, missing := false, false
	for i, snap := range snapshots {
		cur := snap.Currency
		fmt.Fprintf(&sb, "  %-12s %-4s %18s %18s %18s %18s %18s\n",
			snap.RecordedAt.Format("2006-01-02"), currencyCode(cur),
			formatMoney(snap.TotalNetWorth, cur),
			formatMoney(snap.CashBalance, cur),
			formatMoney(snap.FixedDepositBalance, cur),
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

// downsampleSnapshots keeps the last snapshot of each week (ISO) or month in
// snapshots, which are in date order with one per day; "day" keeps them all.
func downsampleSnapshots(snapshots []models.PortfolioSnapshot, groupBy string) []models.PortfolioSnapshot {
	if groupBy == "day" {
		return snapshots
	}
	// A change of reporting currency starts a new bucket, so rows in different
	// currencies are never merged.
	bucket := func(snap *models.PortfolioSnapshot) string {
		t := snap.RecordedAt.UTC()
		if groupBy == "week" {
			year, week := t.ISOWeek()
			return fmt.Sprintf("%d-W%02d %s", year, week, snap.Currency)
		}
		return t.Format("2006-01") + " " + snap.Currency
	}
	var out []models.PortfolioSnapshot
	for i := range snapshots {
		if n := len(out); n > 0 && bucket(&out[n-1]) == bucket(&snapshots[i]) {
			out[n-1] = snapshots[i]
			continue
		}
		out = append(out, snapshots[i])
	}
	return out
}
