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
		mcp.NewTool("get_portfolio",
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

	portfolio, err := s.services.Investments.GetPortfolio(userID)
	if err != nil {
		return mcp.NewToolResultError("failed to get portfolio"), nil
	}

	var sb strings.Builder
	sb.WriteString("Portfolio Summary:\n\n")
	sb.WriteString(fmt.Sprintf("  Total Value:          %s\n", formatCents(portfolio.TotalValue)))
	sb.WriteString(fmt.Sprintf("  Total Cost Basis:     %s\n", formatCents(portfolio.TotalCostBasis)))
	sb.WriteString(fmt.Sprintf("  Unrealized Gain/Loss: %s (%.2f%%)\n", formatCents(portfolio.TotalGainLoss), portfolio.GainLossPct))
	sb.WriteString(fmt.Sprintf("  Realized Gain/Loss:   %s\n", formatCents(portfolio.TotalRealizedGainLoss)))

	if len(portfolio.HoldingsByType) > 0 {
		sb.WriteString("\n  By Asset Type:\n")
		for assetType, summary := range portfolio.HoldingsByType {
			sb.WriteString(fmt.Sprintf("    %-12s %12s  (%d holdings)\n",
				assetType, formatCents(summary.Value), summary.Count))
		}
	}

	// Also list individual holdings
	page := pagination.PageRequest{Page: 1, PageSize: 100}
	holdings, err := s.services.Investments.GetAllInvestments(userID, services.InvestmentStatusOpen, page)
	if err == nil && len(holdings.Data) > 0 {
		sb.WriteString("\n  Holdings:\n")
		sb.WriteString(fmt.Sprintf("    %-8s %-20s %10s %12s %12s\n",
			"Symbol", "Name", "Qty", "Cost Basis", "Cur. Value"))
		sb.WriteString(fmt.Sprintf("    %-8s %-20s %10s %12s %12s\n",
			"------", "----", "---", "----------", "----------"))

		for _, inv := range holdings.Data {
			symbol := inv.Security.Symbol
			name := inv.Security.Name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			currentValue := int64(math.Round(inv.Quantity * float64(inv.CurrentPrice)))
			sb.WriteString(fmt.Sprintf("    %-8s %-20s %10.2f %12s %12s\n",
				symbol, name, inv.Quantity, formatCents(inv.CostBasis), formatCents(currentValue)))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
