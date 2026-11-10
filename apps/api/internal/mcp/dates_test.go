package mcp

import (
	"strings"
	"testing"
	"time"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// Dates are stored as UTC midnight. A driver that returns them in the server's
// local zone must not make a west-of-UTC host print the previous day.
func TestListTransactionsPrintsTheStoredUTCDate(t *testing.T) {
	west := time.FixedZone("UTC-8", -8*3600)
	stored := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC).In(west) // 2026-11-08 16:00 local

	s := &Server{services: Services{Transactions: mockTransactions{
		listFn: func(string, pagination.PageRequest, services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
			return &pagination.PageResponse[models.Transaction]{
				Data: []models.Transaction{{
					Type: models.TransactionTypeExpense, Amount: 1250, Date: stored, Description: "Lunch",
					Account: models.Account{Name: "Cash", Currency: "MYR"},
				}},
				Page: 1, TotalPages: 1, TotalItems: 1,
			}, nil
		},
	}}}
	res, err := s.handleListTransactions(authCtx("read:transactions"), callReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	out := resultText(t, res)
	if !strings.Contains(out, "2026-11-09") || strings.Contains(out, "2026-11-08") {
		t.Errorf("date printed in the local zone:\n%s", out)
	}
}
