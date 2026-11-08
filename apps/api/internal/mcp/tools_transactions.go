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
			mcp.WithString("type", mcp.Description("Filter by type: 'income', 'expense', 'transfer', 'investment', 'reimbursement' (a repayment received for a shared expense), or 'opening_balance' (an account's starting balance, not income)")),
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
			models.TransactionTypeInvestment, models.TransactionTypeReimbursement, models.TransactionTypeOpeningBalance:
			t := models.TransactionType(v)
			filter.Type = &t
		default:
			return mcp.NewToolResultError("invalid type, expected one of: income, expense, transfer, investment, reimbursement, opening_balance"), nil
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
			transactionAmount(t),
			category,
			untrusted(t.Description))
		if note := transactionNote(*t); note != "" {
			sb.WriteString("    " + note + "\n")
		}
		fmt.Fprintf(&sb, "    ID: %s  Account: %s\n", t.ID, transactionAccounts(t))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// transactionAmount renders a transaction's amount in its account's currency.
// A transfer shows both legs when they differ in currency or amount:
// "MYR 4,460.00 → USD 1,100.00 (rate 0.2466)".
func transactionAmount(t *models.Transaction) string {
	out := formatMoney(t.Amount, t.Account.Currency)
	if t.Type != models.TransactionTypeTransfer || t.ToAmount == nil {
		return out
	}
	toCurrency := t.Account.Currency
	if t.ToAccount != nil {
		toCurrency = t.ToAccount.Currency
	}
	out += " → " + formatMoney(t.EffectiveToAmount(), toCurrency)
	if t.ImpliedRate != "" {
		out += " (rate " + trimRate(t.ImpliedRate) + ")"
	}
	return out
}

// transactionAccounts names a transaction's account, or "From → To" for a
// transfer.
func transactionAccounts(t *models.Transaction) string {
	from := untrusted(t.Account.Name)
	if t.Type == models.TransactionTypeTransfer && t.ToAccount != nil {
		return from + " → " + untrusted(t.ToAccount.Name)
	}
	return from
}

// trimRate drops the trailing zeros of a decimal rate string ("0.2466000000"
// becomes "0.2466").
func trimRate(rate string) string {
	if !strings.Contains(rate, ".") {
		return rate
	}
	return strings.TrimSuffix(strings.TrimRight(rate, "0"), ".")
}

// transactionNote returns an extra line describing shared-expense semantics
// for a transaction, or "" when there is nothing to add. A set personal_amount
// means only that share counts as the user's spending; reimbursements are
// repayments received, not income.
func transactionNote(t models.Transaction) string {
	switch {
	case t.Type == models.TransactionTypeReimbursement:
		return "Repayment received for a shared expense (not income)"
	case t.Type == models.TransactionTypeOpeningBalance:
		return "Opening balance of the account (not income)"
	case t.PersonalAmount != nil:
		return sharedExpenseNote(*t.PersonalAmount, t.Amount, t.Account.Currency)
	}
	return ""
}

// sharedExpenseNote describes the user's share of a shared expense.
func sharedExpenseNote(personal, amount int64, currency string) string {
	return fmt.Sprintf("Shared expense: your share %s of %s", formatMoney(personal, currency), formatMoney(amount, currency))
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
	fmt.Fprintf(&sb, "Spending by Category (%s to %s, in %s):\n\n", fromStr, toStr, currencyCode(result.Currency))
	for _, item := range result.Items {
		fmt.Fprintf(&sb, "  %-32s %12s\n", untrusted(item.CategoryName), formatMoney(item.Total, result.Currency))
	}
	fmt.Fprintf(&sb, "\n  Total in %s: %s\n", currencyCode(result.Currency), formatMoney(result.TotalSpent, result.Currency))
	sb.WriteString(fxNotes(result.FXMissing, result.Unconverted))

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
	fmt.Fprintf(&sb, "Monthly Summary (last %d months, in %s):\n\n", months, currencyCode(items[0].Currency))
	fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n", "Month", "Income", "Expenses", "Net")
	fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n", "-----", "------", "--------", "---")

	missing := false
	for _, item := range items {
		net := item.Income - item.Expenses
		fmt.Fprintf(&sb, "  %-10s %12s %12s %12s\n",
			item.Month, formatMoney(item.Income, item.Currency), formatMoney(item.Expenses, item.Currency), formatMoney(net, item.Currency))
		missing = missing || item.FXMissing
	}
	sb.WriteString(fxNotes(missing, nil))

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
	fmt.Fprintf(&sb, "Daily Spending (%s to %s, in %s):\n\n", fromStr, toStr, currencyCode(items[0].Currency))
	missing := false
	for _, item := range items {
		fmt.Fprintf(&sb, "  %s  %12s\n", item.Date, formatMoney(item.Total, item.Currency))
		missing = missing || item.FXMissing
	}
	sb.WriteString(fxNotes(missing, nil))

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
	fmt.Fprintf(&sb, "Daily Summary (%s to %s, in %s):\n\n", fromStr, toStr, currencyCode(items[0].Currency))
	missing := false
	for _, item := range items {
		net := item.Income - item.Expenses
		fmt.Fprintf(&sb, "  %s  +%s  -%s  net %s\n",
			item.Date, formatMoney(item.Income, item.Currency), formatMoney(item.Expenses, item.Currency), formatMoney(net, item.Currency))
		missing = missing || item.FXMissing
	}
	sb.WriteString(fxNotes(missing, nil))

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
	fmt.Fprintf(&sb, "Top Expenses (%s to %s, ranked in %s):\n\n", fromStr, toStr, currencyCode(result.Currency))
	for i := range result.Items {
		item := &result.Items[i]
		// Items are ranked by the effective amount (the user's own share), so show that.
		effective := item.Amount
		if item.PersonalAmount != nil {
			effective = *item.PersonalAmount
		}
		var amountBase *int64
		if !item.FXMissing {
			amountBase = &item.AmountBase
		}
		fmt.Fprintf(&sb, "  %2d. %-32s %12s  [%s / %s]  %s\n",
			i+1, untrusted(item.Description), moneyWithBase(effective, item.Currency, amountBase, result.Currency),
			untrusted(item.CategoryName), untrusted(item.AccountName), item.Date.Format("2006-01-02"))
		if item.PersonalAmount != nil {
			sb.WriteString("      " + sharedExpenseNote(*item.PersonalAmount, item.Amount, item.Currency) + "\n")
		}
	}
	if result.FXMissing {
		sb.WriteString("Note: expenses with no exchange rate are not converted and rank last\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
