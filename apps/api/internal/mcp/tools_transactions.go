package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

func (s *Server) registerTransactionTools() {
	s.mcp.AddTool(
		mcp.NewTool("list_transactions",
			mcp.WithDescription("Search and list transactions with filters. Returns date, description, amount, category, and account."),
			mcp.WithString("account_id", mcp.Description("Filter to a specific account ID")),
			mcp.WithString("category_id", mcp.Description("Filter to a specific category ID")),
			mcp.WithString("type", mcp.Description("Filter by type: 'income', 'expense', or 'transfer'")),
			mcp.WithString("from_date", mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Description("End date (YYYY-MM-DD)")),
			mcp.WithNumber("min_amount", mcp.Description("Minimum amount in dollars")),
			mcp.WithNumber("max_amount", mcp.Description("Maximum amount in dollars")),
			mcp.WithNumber("page", mcp.Description("Page number (default 1)")),
			mcp.WithNumber("page_size", mcp.Description("Items per page (default 20, max 100)")),
		),
		s.handleListTransactions,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_spending_by_category",
			mcp.WithDescription("Get total spending broken down by category for a date range. Great for understanding where money goes."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetSpendingByCategory,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_monthly_summary",
			mcp.WithDescription("Get monthly income vs expenses summary for the last N months. Shows net savings trend."),
			mcp.WithNumber("months", mcp.Description("Number of months to look back (default 6)")),
		),
		s.handleGetMonthlySummary,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_daily_spending",
			mcp.WithDescription("Get daily expense totals for a date range. Useful for spotting spending patterns."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetDailySpending,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_daily_summary",
			mcp.WithDescription("Get daily income and expense totals for a date range. Useful for a calendar-style view of cash flow."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetDailySummary,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_top_expenses",
			mcp.WithDescription("Get the largest expense transactions for a date range, ordered by amount descending."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10, max 100)")),
			mcp.WithString("category_id", mcp.Description("Filter to a single category ID")),
		),
		s.handleGetTopExpenses,
	)
}

func (s *Server) handleListTransactions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
		return denied, nil
	}

	args := req.GetArguments()

	pageNum := 1
	if v, ok := args["page"].(float64); ok && v > 0 {
		pageNum = int(v)
	}
	pageSize := 20
	if v, ok := args["page_size"].(float64); ok && v > 0 {
		pageSize = int(v)
		if pageSize > 100 {
			pageSize = 100
		}
	}
	page := pagination.PageRequest{Page: pageNum, PageSize: pageSize}

	filter := services.TransactionFilter{}

	if v, ok := args["account_id"].(string); ok && v != "" {
		filter.AccountID = &v
	}
	if v, ok := args["category_id"].(string); ok && v != "" {
		filter.CategoryID = &v
	}
	if v, ok := args["type"].(string); ok && v != "" {
		switch models.TransactionType(v) {
		case models.TransactionTypeIncome, models.TransactionTypeExpense, models.TransactionTypeTransfer:
			t := models.TransactionType(v)
			filter.Type = &t
		default:
			return mcp.NewToolResultError("invalid type, expected one of: income, expense, transfer"), nil
		}
	}
	if v, ok := args["from_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.FromDate = &t
		}
	}
	if v, ok := args["to_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.ToDate = &t
		}
	}
	if v, ok := args["min_amount"].(float64); ok {
		cents := dollarsToCents(v)
		filter.MinAmount = &cents
	}
	if v, ok := args["max_amount"].(float64); ok {
		cents := dollarsToCents(v)
		filter.MaxAmount = &cents
	}

	result, err := s.services.Transactions.GetUserTransactions(userID, page, filter)
	if err != nil {
		return mcp.NewToolResultError("failed to list transactions"), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No transactions found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Transactions (page %d of %d, %d total):\n\n", result.Page, result.TotalPages, result.TotalItems))

	for _, t := range result.Data {
		category := "uncategorized"
		if t.Category != nil {
			category = t.Category.Name
		}
		sb.WriteString(fmt.Sprintf("  %s  %-10s  %12s  %-20s  %s\n",
			t.Date.Format("2006-01-02"),
			t.Type,
			formatCents(t.Amount),
			category,
			t.Description,
		))
		sb.WriteString(fmt.Sprintf("    ID: %s  Account: %s\n", t.ID, t.Account.Name))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetSpendingByCategory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
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

	result, err := s.services.Transactions.GetSpendingByCategory(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get spending by category"), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Spending by Category (%s to %s):\n\n", fromStr, toStr))
	for _, item := range result.Items {
		sb.WriteString(fmt.Sprintf("  %-30s %12s\n", item.CategoryName, formatCents(item.Total)))
	}
	sb.WriteString(fmt.Sprintf("\n  Total: %s\n", formatCents(result.TotalSpent)))

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetMonthlySummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
		return denied, nil
	}

	args := req.GetArguments()
	months := 6
	if v, ok := args["months"].(float64); ok && v > 0 {
		months = int(v)
	}

	items, err := s.services.Transactions.GetMonthlySummary(userID, months)
	if err != nil {
		return mcp.NewToolResultError("failed to get monthly summary"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No data for the requested period."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Monthly Summary (last %d months):\n\n", months))
	sb.WriteString(fmt.Sprintf("  %-10s %12s %12s %12s\n", "Month", "Income", "Expenses", "Net"))
	sb.WriteString(fmt.Sprintf("  %-10s %12s %12s %12s\n", "-----", "------", "--------", "---"))

	for _, item := range items {
		net := item.Income - item.Expenses
		sb.WriteString(fmt.Sprintf("  %-10s %12s %12s %12s\n",
			item.Month, formatCents(item.Income), formatCents(item.Expenses), formatCents(net)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetDailySpending(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
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

	items, err := s.services.Transactions.GetDailySpending(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get daily spending"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No spending data for the requested period."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daily Spending (%s to %s):\n\n", fromStr, toStr))
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("  %s  %12s\n", item.Date, formatCents(item.Total)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetDailySummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
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

	items, err := s.services.Transactions.GetDailySummary(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get daily summary"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No data for the requested period."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daily Summary (%s to %s):\n\n", fromStr, toStr))
	for _, item := range items {
		net := item.Income - item.Expenses
		sb.WriteString(fmt.Sprintf("  %s  +%s  -%s  net %s\n",
			item.Date, formatCents(item.Income), formatCents(item.Expenses), formatCents(net)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetTopExpenses(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:transactions"); denied != nil {
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

	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
		if limit > 100 {
			limit = 100
		}
	}

	var categoryID *string
	if v, ok := args["category_id"].(string); ok && v != "" {
		categoryID = &v
	}

	result, err := s.services.Transactions.GetTopExpenses(userID, from, to, limit, categoryID)
	if err != nil {
		return mcp.NewToolResultError("failed to get top expenses"), nil
	}

	if len(result.Items) == 0 {
		return mcp.NewToolResultText("No expenses found for the requested period."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Top Expenses (%s to %s):\n\n", fromStr, toStr))
	for i, item := range result.Items {
		sb.WriteString(fmt.Sprintf("  %2d. %-30s %12s  [%s / %s]  %s\n",
			i+1, item.Description, formatCents(item.Amount), item.CategoryName, item.AccountName, item.Date.Format("2006-01-02")))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
