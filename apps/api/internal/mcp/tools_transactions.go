package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

func (s *Server) registerTransactionTools() {
	s.mcp.AddTool(
		readTool("list_transactions",
			mcp.WithDescription("Search and list transactions with filters. Returns date, description, amount, category, and account."),
			mcp.WithString("account_id", mcp.Description("Filter to a specific account ID")),
			mcp.WithString("category_id", mcp.Description("Filter to a specific category ID")),
			mcp.WithString("type", mcp.Description("Filter by type: 'income', 'expense', 'transfer', 'investment', or 'reimbursement' (a repayment received for a shared expense)")),
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
		readTool("get_spending_by_category",
			mcp.WithDescription("Get total spending broken down by category for a date range. Great for understanding where money goes."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetSpendingByCategory,
	)

	s.mcp.AddTool(
		readTool("get_monthly_summary",
			mcp.WithDescription("Get monthly income vs expenses summary for the last N months. Shows net savings trend."),
			mcp.WithNumber("months", mcp.Description("Number of months to look back (default 6, 1 to 24)")),
		),
		s.handleGetMonthlySummary,
	)

	s.mcp.AddTool(
		readTool("get_daily_spending",
			mcp.WithDescription("Get daily expense totals for a date range of at most 366 days. Useful for spotting spending patterns."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetDailySpending,
	)

	s.mcp.AddTool(
		readTool("get_daily_summary",
			mcp.WithDescription("Get daily income and expense totals for a date range of at most 366 days. Useful for a calendar-style view of cash flow."),
			mcp.WithString("from_date", mcp.Required(), mcp.Description("Start date (YYYY-MM-DD)")),
			mcp.WithString("to_date", mcp.Required(), mcp.Description("End date (YYYY-MM-DD)")),
		),
		s.handleGetDailySummary,
	)

	s.mcp.AddTool(
		readTool("get_top_expenses",
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

	page := parsePage(args)

	filter := services.TransactionFilter{}

	if v, ok := args["account_id"].(string); ok && v != "" {
		filter.AccountID = &v
	}
	if v, ok := args["category_id"].(string); ok && v != "" {
		filter.CategoryID = &v
	}
	if v, ok := args["type"].(string); ok && v != "" {
		switch models.TransactionType(v) {
		case models.TransactionTypeIncome, models.TransactionTypeExpense, models.TransactionTypeTransfer,
			models.TransactionTypeInvestment, models.TransactionTypeReimbursement:
			t := models.TransactionType(v)
			filter.Type = &t
		default:
			return mcp.NewToolResultError("invalid type, expected one of: income, expense, transfer, investment, reimbursement"), nil
		}
	}
	var res *mcp.CallToolResult
	if filter.FromDate, res = optionalArgDate(args, "from_date"); res != nil {
		return res, nil
	}
	if filter.ToDate, res = optionalArgDate(args, "to_date"); res != nil {
		return res, nil
	}
	if filter.FromDate != nil && filter.ToDate != nil {
		if res := checkRange(*filter.FromDate, *filter.ToDate, 0); res != nil {
			return res, nil
		}
	}
	if filter.MinAmount, res = amountFilterArg(args, "min_amount"); res != nil {
		return res, nil
	}
	if filter.MaxAmount, res = amountFilterArg(args, "max_amount"); res != nil {
		return res, nil
	}

	result, err := s.services.Transactions.GetUserTransactions(userID, page, filter)
	if err != nil {
		return mcp.NewToolResultError("failed to list transactions"), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No transactions found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Transactions (page %d of %d, %d total):\n\n", result.Page, result.TotalPages, result.TotalItems)

	for i := range result.Data {
		t := &result.Data[i]
		category := "uncategorized"
		if t.Category != nil {
			category = untrusted(t.Category.Name)
		}
		fmt.Fprintf(&sb, "  %s  %-10s  %12s  %-22s  %s\n",
			t.Date.Format("2006-01-02"),
			t.Type,
			formatCents(t.Amount),
			category,
			untrusted(t.Description))
		if note := transactionNote(*t); note != "" {
			sb.WriteString("    " + note + "\n")
		}
		fmt.Fprintf(&sb, "    ID: %s  Account: %s\n", t.ID, untrusted(t.Account.Name))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// transactionNote returns an extra line describing shared-expense semantics
// for a transaction, or "" when there is nothing to add. A set personal_amount
// means only that share counts as the user's spending; reimbursements are
// repayments received, not income.
func transactionNote(t models.Transaction) string {
	switch {
	case t.Type == models.TransactionTypeReimbursement:
		return "Repayment received for a shared expense (not income)"
	case t.PersonalAmount != nil:
		return sharedExpenseNote(*t.PersonalAmount, t.Amount)
	}
	return ""
}

// sharedExpenseNote describes the user's share of a shared expense.
func sharedExpenseNote(personal, amount int64) string {
	return fmt.Sprintf("Shared expense: your share %s of %s", formatCents(personal), formatCents(amount))
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
	from, to, res := dateRangeArgs(args, maxAggregateRangeDays)
	if res != nil {
		return res, nil
	}
	fromStr, toStr := from.Format("2006-01-02"), to.Format("2006-01-02")

	result, err := s.services.Transactions.GetSpendingByCategory(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get spending by category"), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Spending by Category (%s to %s):\n\n", fromStr, toStr)
	for _, item := range result.Items {
		fmt.Fprintf(&sb, "  %-32s %12s\n", untrusted(item.CategoryName), formatCents(item.Total))
	}
	fmt.Fprintf(&sb, "\n  Total: %s\n", formatCents(result.TotalSpent))

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
	months := monthsArg(args)

	items, err := s.services.Transactions.GetMonthlySummary(userID, months)
	if err != nil {
		return mcp.NewToolResultError("failed to get monthly summary"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No data for the requested period."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Monthly Summary (last %d months):\n\n", months)
	fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n", "Month", "Income", "Expenses", "Net")
	fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n", "-----", "------", "--------", "---")

	for _, item := range items {
		net := item.Income - item.Expenses
		fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n",
			item.Month, formatCents(item.Income), formatCents(item.Expenses), formatCents(net))
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
	from, to, res := dateRangeArgs(args, maxDailyRangeDays)
	if res != nil {
		return res, nil
	}
	fromStr, toStr := from.Format("2006-01-02"), to.Format("2006-01-02")

	items, err := s.services.Transactions.GetDailySpending(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get daily spending"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No spending data for the requested period."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Daily Spending (%s to %s):\n\n", fromStr, toStr)
	for _, item := range items {
		fmt.Fprintf(&sb, "  %s  %12s\n", item.Date, formatCents(item.Total))
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
	from, to, res := dateRangeArgs(args, maxDailyRangeDays)
	if res != nil {
		return res, nil
	}
	fromStr, toStr := from.Format("2006-01-02"), to.Format("2006-01-02")

	items, err := s.services.Transactions.GetDailySummary(userID, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to get daily summary"), nil
	}

	if len(items) == 0 {
		return mcp.NewToolResultText("No data for the requested period."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Daily Summary (%s to %s):\n\n", fromStr, toStr)
	for _, item := range items {
		net := item.Income - item.Expenses
		fmt.Fprintf(&sb, "  %s  +%s  -%s  net %s\n",
			item.Date, formatCents(item.Income), formatCents(item.Expenses), formatCents(net))
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
	from, to, res := dateRangeArgs(args, maxAggregateRangeDays)
	if res != nil {
		return res, nil
	}
	fromStr, toStr := from.Format("2006-01-02"), to.Format("2006-01-02")

	limit := limitArg(args, "limit", 10)

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
	fmt.Fprintf(&sb, "Top Expenses (%s to %s):\n\n", fromStr, toStr)
	for i := range result.Items {
		item := &result.Items[i]
		// Items are ranked by the effective amount (the user's own share), so show that.
		effective := item.Amount
		if item.PersonalAmount != nil {
			effective = *item.PersonalAmount
		}
		fmt.Fprintf(&sb, "  %2d. %-32s %12s  [%s / %s]  %s\n",
			i+1, untrusted(item.Description), formatCents(effective), untrusted(item.CategoryName),
			untrusted(item.AccountName), item.Date.Format("2006-01-02"))
		if item.PersonalAmount != nil {
			sb.WriteString("      " + sharedExpenseNote(*item.PersonalAmount, item.Amount) + "\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
