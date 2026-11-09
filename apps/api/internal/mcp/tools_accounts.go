package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

func (s *Server) registerAccountTools() {
	s.mcp.AddTool(
		readTool("list_accounts",
			mcp.WithDescription("List the active financial accounts (types cash, fixed_deposit, investment, credit_card and debt, which is a loan) with current balances. Each balance is in the account's own currency, with its value in the user's base currency (≈) when that differs; an investment account's balance is the value of its holdings, a credit card or debt balance is the amount owed. Ends with the net worth in the base currency. Limited to the first 100 accounts."),
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
		return readError("list accounts", err), nil
	}

	if len(result.Data) == 0 {
		return mcp.NewToolResultText("No accounts found."), nil
	}

	user, err := s.services.Users.GetUserByID(userID)
	if err != nil {
		return readError("list accounts", err), nil
	}
	base := user.BaseCurrency

	var sb strings.Builder
	fmt.Fprintf(&sb, "Accounts (%d total):\n\n", result.TotalItems)
	var netWorth int64
	var excluded []services.UnconvertedAmount
	stale := false
	for i := range result.Data {
		a := &result.Data[i]
		stale = stale || (a.FXStale && a.BalanceBase != nil)
		status := ""
		if !a.IsActive {
			status = " [inactive]"
		}
		fmt.Fprintf(&sb, "  %-32s %-12s %s%s\n",
			untrusted(a.Name), a.Type, moneyWithBase(a.Balance, a.Currency, a.BalanceBase, base), status)
		fmt.Fprintf(&sb, "    ID: %s\n", a.ID)

		if !a.IsActive {
			continue
		}
		sign := int64(1)
		switch a.Type {
		case models.AccountTypeCash, models.AccountTypeFixedDeposit, models.AccountTypeInvestment:
		case models.AccountTypeDebt, models.AccountTypeCreditCard:
			sign = -1
		default:
			continue
		}
		switch {
		case a.BalanceBase != nil:
			netWorth += sign * *a.BalanceBase
		case strings.EqualFold(a.Currency, base):
			netWorth += sign * a.Balance
		default:
			excluded = append(excluded, services.UnconvertedAmount{Currency: a.Currency, Amount: a.Balance})
		}
	}
	fmt.Fprintf(&sb, "\nNet worth (%s): %s (active cash, fixed-deposit and investment accounts minus credit cards and debts; receivables are not counted)\n",
		strings.ToUpper(base), formatMoney(netWorth, base))
	sb.WriteString(fxNotes(false, excluded))
	if stale {
		sb.WriteString("Note: some exchange rates used are stale\n")
	}
	if result.TotalItems > int64(len(result.Data)) {
		fmt.Fprintf(&sb, "Note: only the first %d of %d accounts are listed and counted in the net worth\n", len(result.Data), result.TotalItems)
	}

	return mcp.NewToolResultText(sb.String()), nil
}
