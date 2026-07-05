package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/pagination"
)

func (s *Server) registerAccountTools() {
	s.mcp.AddTool(
		mcp.NewTool("list_accounts",
			mcp.WithDescription("List all financial accounts (cash, investment, credit card, debt) with current balances. Returns account names, types, balances, and currencies. Limited to the first 100 accounts."),
		),
		s.handleListAccounts,
	)
}

func (s *Server) handleListAccounts(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:accounts"); denied != nil {
		return denied, nil
	}

	page := pagination.PageRequest{Page: 1, PageSize: 100}
	result, err := s.services.Accounts.GetUserAccounts(userID, page)
	if err != nil {
		return mcp.NewToolResultError("failed to list accounts"), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No accounts found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Accounts (%d total):\n\n", result.TotalItems))
	for _, a := range result.Data {
		status := ""
		if !a.IsActive {
			status = " [inactive]"
		}
		sb.WriteString(fmt.Sprintf("  %-30s %-12s %s (%s)%s\n",
			a.Name, a.Type, formatCents(a.Balance), a.Currency, status))
		sb.WriteString(fmt.Sprintf("    ID: %s\n", a.ID))
	}

	return mcp.NewToolResultText(sb.String()), nil
}
