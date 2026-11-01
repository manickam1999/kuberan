package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

func (s *Server) registerInvestmentTools() {
	s.mcp.AddTool(
		readTool("get_portfolio",
			mcp.WithDescription("Get investment portfolio summary including total value, cost basis, gain/loss, breakdown by asset type, and individual holdings."),
		),
		s.handleGetPortfolio,
	)
}

func (s *Server) handleGetPortfolio(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := getUserID(ctx)
	if err != nil {
		return errUnauthorized(), nil
	}
	if denied := requireScope(ctx, "read:investments", "read:portfolio"); denied != nil {
		return denied, nil
	}

	portfolio, err := s.services.Investments.GetPortfolio(userID)
	if err != nil {
		return mcp.NewToolResultError("failed to get portfolio"), nil
	}

	var sb strings.Builder
	sb.WriteString("Portfolio Summary:\n\n")
	fmt.Fprintf(&sb, "  Total Value:          %s\n", formatCents(portfolio.TotalValue))
	fmt.Fprintf(&sb, "  Total Cost Basis:     %s\n", formatCents(portfolio.TotalCostBasis))
	fmt.Fprintf(&sb, "  Unrealized Gain/Loss: %s (%.2f%%)\n", formatCents(portfolio.TotalGainLoss), portfolio.GainLossPct)
	fmt.Fprintf(&sb, "  Realized Gain/Loss:   %s\n", formatCents(portfolio.TotalRealizedGainLoss))

	if len(portfolio.HoldingsByType) > 0 {
		sb.WriteString("\n  By Asset Type:\n")
		for assetType, summary := range portfolio.HoldingsByType {
			fmt.Fprintf(&sb, "    %-12s %12s  (%d holdings)\n",
				assetType, formatCents(summary.Value), summary.Count)
		}
	}

	// Also list individual holdings
	page := pagination.PageRequest{Page: 1, PageSize: 100}
	holdings, err := s.services.Investments.GetAllInvestments(userID, services.InvestmentStatusOpen, page)
	if err == nil && len(holdings.Data) > 0 {
		sb.WriteString("\n  Holdings:\n")
		fmt.Fprintf(&sb, "    %-8s %-25s %10s %12s %12s\n",
			"Symbol", "Name", "Qty", "Cost Basis", "Cur. Value")
		fmt.Fprintf(&sb, "    %-8s %-25s %10s %12s %12s\n",
			"------", "----", "---", "----------", "----------")

		for j := range holdings.Data {
			inv := &holdings.Data[j]
			symbol := cleanText(inv.Security.Symbol, 12)
			name := untrusted(cleanText(inv.Security.Name, 20))
			currentValue := int64(math.Round(inv.Quantity * float64(inv.CurrentPrice)))
			fmt.Fprintf(&sb, "    %-8s %-25s %10.2f %12s %12s\n",
				symbol, name, inv.Quantity, formatCents(inv.CostBasis), formatCents(currentValue))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
