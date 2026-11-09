package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/services"
)

// receivablesScope gates the receivables tools. Receivables are derived from
// expense transactions (a shared expense plus its repayments), so they reuse
// read:transactions rather than introducing a new scope that existing OAuth
// clients and consent screens would not know about.
const receivablesScope = "read:transactions"

func (s *Server) registerReceivableTools() {
	s.mcp.AddTool(
		readTool("list_receivables",
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
		readTool("get_receivables_summary",
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

	page := parsePage(args)

	filter := services.ReceivableFilter{}
	if v, ok := args["status"].(string); ok && v != "" {
		status := models.ReceivableStatus(v)
		if status != services.ReceivableStatusOpen && !models.IsValidReceivableStatus(status) {
			return mcp.NewToolResultError("invalid status, expected one of: pending, partially_paid, settled, written_off, open"), nil
		}
		filter.Status = status
	}
	if v, ok := args["kind"].(string); ok && v != "" {
		kind := models.ReceivableKind(v)
		if !models.IsValidReceivableKind(kind) {
			return mcp.NewToolResultError("invalid kind, expected one of: split, on_behalf, claim, loan, deposit, refund"), nil
		}
		filter.Kind = kind
	}
	counterparty, res := filterTextArg(args, "counterparty")
	if res != nil {
		return res, nil
	}
	// The service normalizes the key the same way stored keys are derived.
	filter.CounterpartyKey = counterparty

	result, err := s.services.Receivables.ListReceivables(userID, page, filter)
	if err != nil {
		return readError("list receivables", err), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No receivables found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Receivables (page %d of %d, %d total):\n\n", result.Page, result.TotalPages, result.TotalItems)

	for i := range result.Data {
		r := &result.Data[i]
		fmt.Fprintf(&sb, "  %s  [%s]  %s  status: %s\n",
			untrusted(r.CounterpartyName), r.Kind, formatMoney(r.Amount, r.Currency), r.Status)
		fmt.Fprintf(&sb, "    paid %s, written off %s, outstanding %s\n",
			formatMoney(r.PaidAmount, r.Currency), formatMoney(r.WrittenOffAmount, r.Currency), formatMoney(r.OutstandingAmount(), r.Currency))
		if r.Transaction != nil {
			fmt.Fprintf(&sb, "    Expense: %s on %s (total %s)\n",
				untrusted(r.Transaction.Description), r.Transaction.Date.Format("2006-01-02"), formatMoney(r.Transaction.Amount, r.Currency))
		}
		fmt.Fprintf(&sb, "    ID: %s\n", r.ID)
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
		return readError("get receivables summary", err), nil
	}

	const note = "Note: receivables are NOT included in net worth; they only count once repaid.\n"

	if summary.TotalOutstanding == 0 && len(summary.ByCounterparty) == 0 {
		return mcp.NewToolResultText("No open receivables.\n" + note), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Open receivables: %s outstanding (total in %s)\n",
		formatMoney(summary.TotalOutstanding, summary.Currency), currencyCode(summary.Currency))
	if summary.FXMissing {
		sb.WriteString("Note: some receivables are excluded from the total (no exchange rate); see the per-currency amounts below\n")
	}
	sb.WriteString(note)

	if len(summary.ByCurrency) > 0 {
		sb.WriteString("\nBy currency (not converted):\n")
		for _, c := range summary.ByCurrency {
			fmt.Fprintf(&sb, "  %s\n", formatMoney(c.Outstanding, c.Currency))
		}
	}

	fmt.Fprintf(&sb, "\nBy counterparty (in %s):\n", currencyCode(summary.Currency))
	for _, c := range summary.ByCounterparty {
		fmt.Fprintf(&sb, "  %-27s %12s  (%d open, oldest %s)\n",
			untrusted(c.CounterpartyName), formatMoney(c.Outstanding, summary.Currency), c.OpenCount, c.OldestDate.Format("2006-01-02"))
	}

	fmt.Fprintf(&sb, "\nBy kind (in %s):\n", currencyCode(summary.Currency))
	for _, k := range summary.ByKind {
		fmt.Fprintf(&sb, "  %-25s %12s  (%d open)\n", k.Kind, formatMoney(k.Outstanding, summary.Currency), k.OpenCount)
	}

	return mcp.NewToolResultText(sb.String()), nil
}
