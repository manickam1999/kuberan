package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/pagination"
)

func (s *Server) registerBudgetTools() {
	s.mcp.AddTool(
		readTool("list_budgets",
			mcp.WithDescription("List budgets with their category, amount (in the budget's currency), and period; paused ones are marked. Limited to the first 100 budgets."),
			mcp.WithBoolean("active_only", mcp.Description("true (default) lists only active budgets; false also lists paused ones")),
		),
		s.handleListBudgets,
	)

	s.mcp.AddTool(
		readTool("get_budget_progress",
			mcp.WithDescription("Get spending progress against a budget for its current month or year, in the budget's currency: budgeted, spent, remaining and percentage used. Spending in other currencies is converted at each day's rate; amounts with no rate are left out and flagged."),
			mcp.WithString("budget_id", mcp.Required(), mcp.Description("The budget ID")),
		),
		s.handleGetBudgetProgress,
	)
}

func (s *Server) handleListBudgets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:budgets"); denied != nil {
		return denied, nil
	}

	page := pagination.PageRequest{Page: 1, PageSize: 100}

	args := req.GetArguments()
	// active_only defaults to true; false lists paused budgets too (a nil
	// filter), not only the paused ones.
	var activeFilter *bool
	if v, ok := args["active_only"].(bool); !ok || v {
		active := true
		activeFilter = &active
	}

	result, err := s.services.Budgets.GetUserBudgets(userID, page, activeFilter, nil)
	if err != nil {
		return readError("list budgets", err), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No budgets found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Budgets (%d total):\n\n", result.TotalItems)
	for i := range result.Data {
		b := &result.Data[i]
		paused := ""
		if !b.IsActive {
			paused = " [paused]"
		}
		fmt.Fprintf(&sb, "  %-27s %12s  %-8s  %s%s\n",
			untrusted(b.Name), formatMoney(b.Amount, b.Currency), b.Period, untrusted(b.Category.Name), paused)
		fmt.Fprintf(&sb, "    ID: %s\n", b.ID)
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetBudgetProgress(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:budgets"); denied != nil {
		return denied, nil
	}

	args := req.GetArguments()
	budgetID, _ := args["budget_id"].(string)
	if budgetID == "" {
		return mcp.NewToolResultError("budget_id is required"), nil
	}

	progress, err := s.services.Budgets.GetBudgetProgress(userID, budgetID)
	if err != nil {
		return readError("get budget progress", err), nil
	}

	var sb strings.Builder
	sb.WriteString("Budget Progress:\n\n")
	fmt.Fprintf(&sb, "  Budgeted:   %s\n", formatMoney(progress.Budgeted, progress.Currency))
	fmt.Fprintf(&sb, "  Spent:      %s\n", formatMoney(progress.Spent, progress.Currency))
	fmt.Fprintf(&sb, "  Remaining:  %s\n", formatMoney(progress.Remaining, progress.Currency))
	fmt.Fprintf(&sb, "  Used:       %.1f%%\n", progress.Percentage)
	if progress.FXMissing {
		fmt.Fprintf(&sb, "Note: spending in other currencies with no exchange rate to %s is excluded\n", currencyCode(progress.Currency))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
