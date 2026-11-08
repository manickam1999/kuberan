package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

func (s *Server) registerInvestmentTools() {
	s.mcp.AddTool(
		readTool("get_portfolio",
			mcp.WithDescription("Get investment portfolio summary in the user's base currency: total value, cost basis, unrealized gain/loss (split into price and FX return), realized gain/loss, breakdown by asset type, and individual holdings (priced in each security's own currency, with base-currency value and gain)."),
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

	base := portfolio.Currency
	var sb strings.Builder
	fmt.Fprintf(&sb, "Portfolio Summary (totals in %s):\n\n", currencyCode(base))
	fmt.Fprintf(&sb, "  Total Value:          %s\n", formatMoney(portfolio.TotalValue, base))
	fmt.Fprintf(&sb, "  Total Cost Basis:     %s\n", formatMoney(portfolio.TotalCostBasis, base))
	fmt.Fprintf(&sb, "  Unrealized Gain/Loss: %s (%.2f%%)\n", formatMoney(portfolio.TotalGainLoss, base), portfolio.GainLossPct)
	fmt.Fprintf(&sb, "    Price return:       %s\n", formatMoney(portfolio.TotalPriceReturn, base))
	fmt.Fprintf(&sb, "    FX return:          %s\n", formatMoney(portfolio.TotalFXReturn, base))
	fmt.Fprintf(&sb, "  Realized Gain/Loss:   %s\n", formatMoney(portfolio.TotalRealizedGainLoss, base))
	for _, u := range portfolio.Unconverted {
		fmt.Fprintf(&sb, "Note: excludes %s of holdings (no exchange rate)\n", formatMoney(u.Value, u.Currency))
	}
	if portfolio.FXStale {
		sb.WriteString("Note: some exchange rates used are stale\n")
	}

	if len(portfolio.HoldingsByType) > 0 {
		sb.WriteString("\n  By Asset Type:\n")
		for assetType, summary := range portfolio.HoldingsByType {
			fmt.Fprintf(&sb, "    %-12s %18s  (%d holdings)\n",
				assetType, formatMoney(summary.Value, base), summary.Count)
		}
	}

	// Also list individual holdings
	page := pagination.PageRequest{Page: 1, PageSize: 100}
	holdings, err := s.services.Investments.GetAllInvestments(userID, services.InvestmentStatusOpen, page)
	if err == nil && len(holdings.Data) > 0 {
		sb.WriteString("\n  Holdings:\n")
		for j := range holdings.Data {
			sb.WriteString(holdingLines(&holdings.Data[j], base))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// holdingLines renders one holding: the quantity at the native price and
// value in the security's currency, then the base-currency value, cost basis
// and unrealized gain (quantity x current_price - cost_basis_base, the same
// formula as the web app). A holding with no FX rate shows no base figures.
func holdingLines(inv *models.Investment, base string) string {
	symbol := cleanText(inv.Security.Symbol, 12)
	name := untrusted(cleanText(inv.Security.Name, 20))
	currency := inv.PriceCurrency
	if currency == "" {
		currency = inv.Currency
	}
	nativeValue := int64(math.Round(inv.Quantity * float64(inv.CurrentPriceNative)))

	var sb strings.Builder
	fmt.Fprintf(&sb, "    %-8s %-25s %10.2f @ %s = %s\n", symbol, name, inv.Quantity,
		formatMoney(inv.CurrentPriceNative, currency), formatMoney(nativeValue, currency))
	if inv.FXMissing {
		fmt.Fprintf(&sb, "      Value in %s: unavailable (no exchange rate); cost basis %s\n",
			currencyCode(base), formatMoney(inv.CostBasisBase, base))
		return sb.String()
	}
	valueBase := int64(math.Round(inv.Quantity * float64(inv.CurrentPrice)))
	fmt.Fprintf(&sb, "      Value %s, cost basis %s, unrealized %s\n",
		formatMoney(valueBase, base), formatMoney(inv.CostBasisBase, base), formatMoney(valueBase-inv.CostBasisBase, base))
	return sb.String()
}
