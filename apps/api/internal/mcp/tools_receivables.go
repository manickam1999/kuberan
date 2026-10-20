package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// receivablesScope gates the receivables tools. Receivables are derived from
// expense transactions (a shared expense plus its repayments), so they reuse
// read:transactions rather than introducing a new scope that existing OAuth
// clients and consent screens would not know about.
const receivablesScope = "read:transactions"

func (s *Server) registerReceivableTools() {
	s.mcp.AddTool(
		mcp.NewTool("list_receivables",
			mcp.WithDescription("List money owed to the user (shared expenses, loans, claims, deposits, refunds), newest expense first. "+
				"Each entry shows counterparty, kind, amount, paid, written off, outstanding, status and the originating expense. "+
				"Receivables are not counted in net worth until repaid."),
			mcp.WithString("status", mcp.Description("Filter by status: 'pending', 'partially_paid', 'settled', 'written_off', or 'open' (pending or partially_paid)")),
			mcp.WithString("kind", mcp.Description("Filter by kind: 'split', 'on_behalf', 'claim', 'loan', 'deposit', or 'refund'")),
			mcp.WithString("counterparty", mcp.Description("Filter by counterparty name (case-insensitive exact match)")),
			mcp.WithNumber("page", mcp.Description("Page number (default 1)")),
			mcp.WithNumber("page_size", mcp.Description("Items per page (default 20, max 100)")),
		),
		s.handleListReceivables,
	)

	s.mcp.AddTool(
		mcp.NewTool("get_receivables_summary",
			mcp.WithDescription("Summarize open receivables (money others still owe the user): total outstanding, by counterparty and by kind. "+
				"Receivables are excluded from net worth; this is informational only."),
		),
		s.handleGetReceivablesSummary,
	)
}

// authorizeReceivables resolves the caller and enforces the receivables scope.
// It returns either the user ID or an MCP error result to return directly.
func authorizeReceivables(ctx context.Context) (string, *mcp.CallToolResult) {
	userID, err := getUserID(ctx)
	if err != nil {
		return "", errUnauthorized()
	}
	if denied := requireScope(ctx, receivablesScope); denied != nil {
		return "", denied
	}
	return userID, nil
}

func (s *Server) handleListReceivables(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, denied := authorizeReceivables(ctx)
	if denied != nil {
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

	filter := services.ReceivableFilter{}
	if v, ok := args["status"].(string); ok && v != "" {
		if v != services.ReceivableStatusOpen && !models.IsValidReceivableStatus(models.ReceivableStatus(v)) {
			return mcp.NewToolResultError("invalid status, expected one of: pending, partially_paid, settled, written_off, open"), nil
		}
		filter.Status = v
	}
	if v, ok := args["kind"].(string); ok && v != "" {
		if !models.IsValidReceivableKind(models.ReceivableKind(v)) {
			return mcp.NewToolResultError("invalid kind, expected one of: split, on_behalf, claim, loan, deposit, refund"), nil
		}
		filter.Kind = v
	}
	if v, ok := args["counterparty"].(string); ok {
		// Same normalization as stored counterparty keys.
		filter.CounterpartyKey = strings.ToLower(strings.Join(strings.Fields(v), " "))
	}

	result, err := s.services.Receivables.ListReceivables(userID, page, filter)
	if err != nil {
		logger.Get().Errorf("MCP list_receivables failed: %v", err)
		return mcp.NewToolResultError("failed to list receivables"), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No receivables found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Receivables (page %d of %d, %d total):\n\n", result.Page, result.TotalPages, result.TotalItems))

	for i := range result.Data {
		r := &result.Data[i]
		sb.WriteString(fmt.Sprintf("  %s  [%s]  %s  status: %s\n",
			r.CounterpartyName, r.Kind, formatCents(r.Amount), r.Status))
		sb.WriteString(fmt.Sprintf("    paid %s, written off %s, outstanding %s\n",
			formatCents(r.PaidAmount), formatCents(r.WrittenOffAmount), formatCents(r.OutstandingAmount())))
		if r.Transaction != nil {
			sb.WriteString(fmt.Sprintf("    Expense: %s on %s (total %s)\n",
				r.Transaction.Description, r.Transaction.Date.Format("2006-01-02"), formatCents(r.Transaction.Amount)))
		}
		sb.WriteString(fmt.Sprintf("    ID: %s\n", r.ID))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetReceivablesSummary(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, denied := authorizeReceivables(ctx)
	if denied != nil {
		return denied, nil
	}

	summary, err := s.services.Receivables.Summary(userID)
	if err != nil {
		logger.Get().Errorf("MCP get_receivables_summary failed: %v", err)
		return mcp.NewToolResultError("failed to get receivables summary"), nil
	}

	const note = "Note: receivables are NOT included in net worth; they only count once repaid.\n"

	if summary.TotalOutstanding == 0 && len(summary.ByCounterparty) == 0 {
		return mcp.NewToolResultText("No open receivables.\n" + note), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Open receivables: %s outstanding\n", formatCents(summary.TotalOutstanding)))
	sb.WriteString(note)

	sb.WriteString("\nBy counterparty:\n")
	for _, c := range summary.ByCounterparty {
		sb.WriteString(fmt.Sprintf("  %-25s %12s  (%d open, oldest %s)\n",
			c.CounterpartyName, formatCents(c.Outstanding), c.OpenCount, c.OldestDate.Format("2006-01-02")))
	}

	sb.WriteString("\nBy kind:\n")
	for _, k := range summary.ByKind {
		sb.WriteString(fmt.Sprintf("  %-25s %12s  (%d open)\n", k.Kind, formatCents(k.Outstanding), k.OpenCount))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
