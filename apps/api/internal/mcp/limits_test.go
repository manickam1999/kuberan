package mcp

import (
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// truncatedInvestments reports more holdings than a page returns.
type truncatedInvestments struct {
	mockInvestments
	total int64
}

func (m truncatedInvestments) GetAllInvestments(_ string, _ services.InvestmentStatus, p pagination.PageRequest) (*pagination.PageResponse[models.Investment], error) {
	resp := pagination.NewPageResponse(m.holdings, p.Page, p.PageSize, m.total)
	return &resp, nil
}

func TestGetPortfolioSaysWhenHoldingsAreTruncated(t *testing.T) {
	inv := truncatedInvestments{
		mockInvestments: mockInvestments{
			portfolio: &services.PortfolioSummary{Currency: "MYR"},
			holdings: []models.Investment{{
				Quantity: 1, CurrentPrice: 100, CurrentPriceNative: 100, PriceCurrency: "MYR", Currency: "MYR",
				Security: models.Security{Symbol: "AAA", Name: "Triple A"},
			}},
		},
		total: 150,
	}
	s := &Server{services: Services{Investments: inv}}
	res, err := s.handleGetPortfolio(authCtx("read:portfolio"), callReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	if out := resultText(t, res); !strings.Contains(out, "only the first 1 of 150 holdings are listed") {
		t.Errorf("a truncated holdings list must say so:\n%s", out)
	}
}

func TestE2EListAccountsFlagsStaleRates(t *testing.T) {
	e := newE2E(t, e2eOptions{write: defaultWriteConfig()})
	// EUR gets a rate, but one observed ten days ago: older than the 168h limit.
	if _, err := e.svc.FX.RecordRates([]services.FXRateInput{
		{Base: "EUR", Quote: "MYR", Rate: mustRate(t, "5"), RecordedAt: time.Now().Add(-10 * 24 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	out := e.as("tok").call("list_accounts", nil).ok(t)
	contains(t, out, "EUR 285.00 (≈ MYR 1,425.00)", "Note: some exchange rates used are stale")
	lacks(t, out, "no exchange rate")
}
