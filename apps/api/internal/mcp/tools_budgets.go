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
			mcp.WithDescription("List all budgets with their category, amount, and period. Limited to the first 100 budgets."),
			mcp.WithBoolean("active_only", mcp.Description("Only show active budgets (default true)")),
		),
		s.handleListBudgets,
	)

	s.mcp.AddTool(
		readTool("get_budget_progress",
			mcp.WithDescription("Get spending progress against a specific budget. Shows budgeted amount, spent, remaining, and percentage used."),
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
	isActive := true
	if v, ok := args["active_only"].(bool); ok {
		isActive = v
	}

	result, err := s.services.Budgets.GetUserBudgets(userID, page, &isActive, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to list budgets"), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No budgets found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Budgets (%d total):\n\n", result.TotalItems)
	for i := range result.Data {
		b := &result.Data[i]
		fmt.Fprintf(&sb, "  %-27s %12s  %-8s  %s\n",
			untrusted(b.Name), formatCents(b.Amount), b.Period, untrusted(b.Category.Name))
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
		return mcp.NewToolResultError("failed to get budget progress"), nil
	}

	var sb strings.Builder
	sb.WriteString("Budget Progress:\n\n")
	fmt.Fprintf(&sb, "  Budgeted:   %s\n", formatCents(progress.Budgeted))
	fmt.Fprintf(&sb, "  Spent:      %s\n", formatCents(progress.Spent))
	fmt.Fprintf(&sb, "  Remaining:  %s\n", formatCents(progress.Remaining))
	fmt.Fprintf(&sb, "  Used:       %.1f%%\n", progress.Percentage)

	return mcp.NewToolResultText(sb.String()), nil
}
