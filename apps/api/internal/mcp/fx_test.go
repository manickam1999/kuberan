package mcp

import (
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
	"kuberan/internal/testutil"
)

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		cents    int64
		currency string
		want     string
	}{
		{123456, "MYR", "MYR 1,234.56"},
		{-1200, "USD", "USD -12.00"},
		{0, "USD", "USD 0.00"},
		{5, "usd", "USD 0.05"},
		{-5, "EUR", "EUR -0.05"},
		{100, "SGD", "SGD 1.00"},
		{99999, "MYR", "MYR 999.99"},
		{100000000, "MYR", "MYR 1,000,000.00"},
		{123456789012, "USD", "USD 1,234,567,890.12"},
		{1050, "", "XXX 10.50"},
		{1050, "U$D\n", "UD 10.50"},
	}
	for _, tt := range tests {
		if got := formatMoney(tt.cents, tt.currency); got != tt.want {
			t.Errorf("formatMoney(%d, %q) = %q, want %q", tt.cents, tt.currency, got, tt.want)
		}
	}
}

// recordRate stores a rate base->quote observed two days ago.
func (e *writeEnv) recordRate(t *testing.T, base, quote, rate string) {
	t.Helper()
	r, err := fx.ParseRate(rate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.services.FX.RecordRates([]services.FXRateInput{{
		Base: base, Quote: quote, Rate: r, RecordedAt: time.Now().Add(-48 * time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
}

// account creates a cash account with a balance in the given currency.
func (e *writeEnv) account(t *testing.T, name, currency string, balance int64) *models.Account {
	t.Helper()
	a := testutil.CreateTestCashAccountWithBalance(t, e.db, e.user.ID, balance)
	if err := e.db.Model(a).Updates(map[string]any{"currency": currency, "name": name}).Error; err != nil {
		t.Fatal(err)
	}
	return a
}

func (e *writeEnv) setBase(t *testing.T, currency string) {
	t.Helper()
	if err := e.db.Model(e.user).Update("base_currency", currency).Error; err != nil {
		t.Fatal(err)
	}
}

// countingFX counts FX lookups made through the MCP layer.
type countingFX struct {
	services.FXServicer
	rateCalls int
}

func (c *countingFX) Rate(from, to string, at time.Time) (*services.FXQuote, error) {
	c.rateCalls++
	return c.FXServicer.Rate(from, to, at)
}

func TestListAccountsPrintsCurrenciesAndNetWorth(t *testing.T) {
	e := setupWriteEnv(t)
	e.setBase(t, "MYR")
	e.recordRate(t, "USD", "MYR", "4.0")
	e.account(t, "Ringgit", "MYR", 100_000)
	e.account(t, "Dollars", "USD", 18_664)
	e.account(t, "Euros", "EUR", 5_000) // no rate
	card := testutil.CreateTestCreditCardAccount(t, e.db, e.user.ID, 20_000)
	e.db.Model(card).Update("currency", "MYR")

	res, err := e.s.handleListAccounts(e.ctx("read:accounts"), callReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	out := wantOK(t, res)
	for _, want := range []string{
		`"Ringgit"`, "MYR 1,000.00",
		`"Dollars"`, "USD 186.64 (≈ MYR 746.56)",
		"EUR 50.00 (no exchange rate)",
		"Net worth (MYR): MYR 1,546.56",
		"Note: excludes EUR 50.00 (no exchange rate)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListTransactionsCrossCurrencyTransfer(t *testing.T) {
	e := setupWriteEnv(t)
	from := e.account(t, "Checking", "MYR", 1_000_000)
	to := e.account(t, "Brokerage cash", "USD", 0)
	toAmount := int64(110_000)
	if _, err := e.s.services.Transactions.CreateTransfer(e.user.ID, from.ID, to.ID, 446_000, &toAmount, "Fund", time.Now(), services.TransactionMeta{}); err != nil {
		t.Fatal(err)
	}
	groceries := e.account(t, "Wallet", "MYR", 0)
	if _, err := e.s.services.Transactions.CreateTransaction(e.user.ID, groceries.ID, nil, models.TransactionTypeExpense, 1_250, "Groceries", time.Now().Add(-time.Hour), services.TransactionMeta{}); err != nil {
		t.Fatal(err)
	}

	res, err := e.s.handleListTransactions(e.ctx("read:transactions"), callReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	out := wantOK(t, res)
	for _, want := range []string{
		"MYR 4,460.00 → USD 1,100.00 (rate 0.2466", // implied rate, trailing zeros trimmed
		`Account: "Checking" → "Brokerage cash"`,   // preloaded names, From → To
		"MYR 12.50",
		`Account: "Wallet"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `Account: ""`) {
		t.Errorf("account name must not be blank:\n%s", out)
	}
}

type mockInvestments struct {
	services.InvestmentServicer
	portfolio *services.PortfolioSummary
	holdings  []models.Investment
}

func (m mockInvestments) GetPortfolio(string) (*services.PortfolioSummary, error) {
	return m.portfolio, nil
}

func (m mockInvestments) GetAllInvestments(_ string, _ services.InvestmentStatus, p pagination.PageRequest) (*pagination.PageResponse[models.Investment], error) {
	resp := pagination.NewPageResponse(m.holdings, p.Page, p.PageSize, int64(len(m.holdings)))
	return &resp, nil
}

func TestGetPortfolioPrintsCurrencies(t *testing.T) {
	inv := mockInvestments{
		portfolio: &services.PortfolioSummary{
			TotalValue: 757_110, TotalCostBasis: 700_000, TotalGainLoss: 57_110, GainLossPct: 8.16,
			TotalPriceReturn: 50_000, TotalFXReturn: 7_110, TotalRealizedGainLoss: 1_000,
			Currency: "MYR", Unconverted: []services.UnconvertedValue{{Currency: "EUR", Value: 12_000}},
			HoldingsByType: map[models.AssetType]services.TypeSummary{"stock": {Value: 757_110, Count: 1}},
		},
		holdings: []models.Investment{{
			Quantity: 10, CostBasisBase: 700_000, CurrentPrice: 75_711, CurrentPriceNative: 18_664,
			PriceCurrency: "USD", Currency: "USD",
			Security: models.Security{Symbol: "VOO", Name: "Vanguard S&P 500"},
		}, {
			Quantity: 2, CostBasisBase: 1_000, CurrentPriceNative: 500, PriceCurrency: "EUR", Currency: "EUR", FXMissing: true,
			Security: models.Security{Symbol: "EUX", Name: "Euro thing"},
		}},
	}
	s := &Server{services: Services{Investments: inv}}
	res, err := s.handleGetPortfolio(authCtx("read:investments"), callReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	out := wantOK(t, res)
	for _, want := range []string{
		"totals in MYR", "Total Value:          MYR 7,571.10", "Price return:       MYR 500.00", "FX return:          MYR 71.10",
		"Note: excludes EUR 120.00 of holdings (no exchange rate)",
		"10.00 @ USD 186.64 = USD 1,866.40", // native price and value
		"Value MYR 7,571.10, cost basis MYR 7,000.00, unrealized MYR 571.10",
		"Value in MYR: unavailable (no exchange rate)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

type mockSnapshots struct {
	services.PortfolioSnapshotServicer
	snaps []models.PortfolioSnapshot
}

func (m mockSnapshots) GetGroupedSnapshots(string, time.Time, time.Time, string) ([]models.PortfolioSnapshot, error) {
	return m.snaps, nil
}

func TestNetWorthHistoryPrintsSnapshotCurrency(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	s := &Server{services: Services{Snapshots: mockSnapshots{snaps: []models.PortfolioSnapshot{
		{RecordedAt: day(1), Currency: "MYR", TotalNetWorth: 1_000_000, CashBalance: 600_000, InvestmentValue: 500_000, DebtBalance: 100_000},
		{RecordedAt: day(2), Currency: "USD", TotalNetWorth: 250_000, FXMissing: true},
	}}}}
	res, err := s.handleGetNetWorthHistory(authCtx("read:snapshots"), callReq(map[string]any{
		"from_date": "2026-01-01", "to_date": "2026-01-31",
	}))
	if err != nil {
		t.Fatal(err)
	}
	out := wantOK(t, res)
	for _, want := range []string{"MYR 10,000.00", "USD 2,500.00", "reporting currency changed", "fx_missing"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

type mockAnalytics struct {
	services.TransactionServicer
}

func (mockAnalytics) GetSpendingByCategory(_ string, from, to time.Time) (*services.SpendingByCategory, error) {
	return &services.SpendingByCategory{
		Items:      []services.SpendingByCategoryItem{{CategoryName: "Food", Total: 50_000}},
		TotalSpent: 50_000, FromDate: from, ToDate: to, Currency: "MYR",
		FXMissing: true, Unconverted: []services.UnconvertedAmount{{Currency: "USD", Amount: 12_000}},
	}, nil
}

func (mockAnalytics) GetTopExpenses(string, time.Time, time.Time, int, *string) (*services.TopExpenses, error) {
	return &services.TopExpenses{Currency: "MYR", Items: []services.TopExpenseItem{
		{Description: "Flight", Amount: 20_000, Currency: "USD", AmountBase: 80_000, CategoryName: "Travel", AccountName: "Card", Date: time.Now()},
		{Description: "Odd", Amount: 100, Currency: "EUR", FXMissing: true, CategoryName: "Misc", AccountName: "Card", Date: time.Now()},
	}, FXMissing: true}, nil
}

func TestAnalyticsToolsPrintCurrencyAndFXNotes(t *testing.T) {
	s := &Server{services: Services{Transactions: mockAnalytics{}}}
	args := map[string]any{"from_date": "2026-10-01", "to_date": "2026-10-31"}

	res, _ := s.handleGetSpendingByCategory(authCtx("read:transactions"), callReq(args))
	out := wantOK(t, res)
	for _, want := range []string{"in MYR", "MYR 500.00", "Total in MYR: MYR 500.00", "Note: excludes USD 120.00 (no exchange rate)"} {
		if !strings.Contains(out, want) {
			t.Errorf("spending output missing %q:\n%s", want, out)
		}
	}

	res, _ = s.handleGetTopExpenses(authCtx("read:transactions"), callReq(args))
	out = wantOK(t, res)
	for _, want := range []string{"USD 200.00 (≈ MYR 800.00)", "EUR 1.00 (no exchange rate)", "rank last"} {
		if !strings.Contains(out, want) {
			t.Errorf("top expenses output missing %q:\n%s", want, out)
		}
	}
}

func TestBudgetToolsPrintBudgetCurrency(t *testing.T) {
	e := setupWriteEnv(t)
	cat := testutil.CreateTestCategory(t, e.db, e.user.ID, models.CategoryTypeExpense)
	b, err := e.s.services.Budgets.CreateBudget(e.user.ID, cat.ID, "Food", 50_000, models.BudgetPeriodMonthly)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := e.s.handleListBudgets(e.ctx("read:budgets"), callReq(nil))
	if out := wantOK(t, res); !strings.Contains(out, "USD 500.00") {
		t.Errorf("list_budgets missing currency:\n%s", out)
	}
	res, _ = e.s.handleGetBudgetProgress(e.ctx("read:budgets"), callReq(map[string]any{"budget_id": b.ID}))
	if out := wantOK(t, res); !strings.Contains(out, "Budgeted:   USD 500.00") || !strings.Contains(out, "Remaining:  USD 500.00") {
		t.Errorf("get_budget_progress missing currency:\n%s", out)
	}
}

func TestWriteCeilingIsEvaluatedInBaseCurrency(t *testing.T) {
	const scope = "write:transactions"
	// Base currency USD, ceiling USD 10,000.00. 1 MYR = 0.25 USD.
	create := func(e *writeEnv, acct *models.Account, amount float64) *mcpgo.CallToolResult {
		return e.call(t, e.ctx(scope), "create_transaction", map[string]any{
			"date": today(), "account_id": acct.ID, "type": "expense", "amount": amount,
		})
	}

	t.Run("a foreign amount under the ceiling after conversion is allowed", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.0")
		myr := e.account(t, "Ringgit", "MYR", 0)
		// MYR 30,000 = USD 7,500: over the ceiling as a bare number, under it in USD.
		wantOK(t, create(e, myr, 30_000))
	})

	t.Run("a foreign amount over the ceiling after conversion is refused", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.0")
		myr := e.account(t, "Ringgit", "MYR", 0)
		// MYR 50,000 = USD 12,500.
		wantError(t, create(e, myr, 50_000), "amount of MYR 50,000.00 (≈ USD 12,500.00) exceeds the per-change limit of USD 10,000.00")
		if e.txnCount(t) != 0 {
			t.Error("a refused write must not create a transaction")
		}
	})

	t.Run("a stronger foreign currency is compared after conversion", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.setBase(t, "MYR")
		e.recordRate(t, "USD", "MYR", "4.0")
		usd := e.account(t, "Dollars", "USD", 0)
		// Ceiling MYR 10,000.00 = USD 2,500: USD 3,000 is under 10,000 as a bare number only.
		wantError(t, create(e, usd, 3_000), "exceeds the per-change limit of MYR 10,000.00")
		wantOK(t, create(e, usd, 2_000))
	})

	t.Run("no rate rejects the write", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		eur := e.account(t, "Euros", "EUR", 0)
		wantError(t, create(e, eur, 1), "no exchange rate from EUR to USD")
		if e.txnCount(t) != 0 {
			t.Error("a refused write must not create a transaction")
		}
	})

	t.Run("a same-currency amount makes no FX query", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		counter := &countingFX{FXServicer: e.s.services.FX}
		e.s.services.FX = counter
		usd := e.account(t, "Dollars", "USD", 0)
		wantOK(t, create(e, usd, 12.50))
		wantError(t, create(e, usd, 10_000.01), "per-change limit of USD 10,000.00")
		if counter.rateCalls != 0 {
			t.Errorf("FX consulted %d times for same-currency amounts, want 0", counter.rateCalls)
		}
	})

	t.Run("without an FX service a foreign amount is refused", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.s.services.FX = nil
		myr := e.account(t, "Ringgit", "MYR", 0)
		wantError(t, create(e, myr, 1), "no exchange rate")
	})
}

func TestCreateTransferToolCrossCurrency(t *testing.T) {
	const scope = "write:transactions"
	transfer := func(e *writeEnv, from, to *models.Account, args map[string]any) *mcpgo.CallToolResult {
		args["date"] = today()
		args["from_account_id"], args["to_account_id"] = from.ID, to.ID
		return e.call(t, e.ctx(scope), "create_transfer", args)
	}

	t.Run("a missing to_amount is refused with the market suggestion", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.4")
		usd := e.account(t, "USD cash", "USD", 100_000)
		myr := e.account(t, "MYR cash", "MYR", 0)
		res := transfer(e, usd, myr, map[string]any{"amount": 100.0})
		wantError(t, res, "to_amount is required")
		wantError(t, res, "MYR 440.00")
		wantError(t, res, "rate 4.4")
		if e.balance(t, usd.ID) != 100_000 || e.balance(t, myr.ID) != 0 || e.auditCount(t, "MCP_CREATE_TRANSFER") != 0 {
			t.Error("a refused transfer must change nothing")
		}
	})

	t.Run("a missing to_amount without a rate names the currency only", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		usd := e.account(t, "USD cash", "USD", 100_000)
		eur := e.account(t, "EUR cash", "EUR", 0)
		res := transfer(e, usd, eur, map[string]any{"amount": 100.0})
		wantError(t, res, "to_amount is required")
		if strings.Contains(resultText(t, res), "suggests") {
			t.Errorf("no rate, so no suggestion expected: %s", resultText(t, res))
		}
	})

	t.Run("with to_amount both balances move", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.4")
		usd := e.account(t, "USD cash", "USD", 100_000)
		myr := e.account(t, "MYR cash", "MYR", 0)
		text := wantOK(t, transfer(e, usd, myr, map[string]any{"amount": 100.0, "to_amount": 441.5}))
		if !strings.Contains(text, "USD 100.00 → MYR 441.50 (rate 4.415)") {
			t.Errorf("unexpected output: %s", text)
		}
		if e.balance(t, usd.ID) != 90_000 || e.balance(t, myr.ID) != 44_150 {
			t.Errorf("balances = %d / %d, want 90000 / 44150", e.balance(t, usd.ID), e.balance(t, myr.ID))
		}
		if e.auditCount(t, "MCP_CREATE_TRANSFER") != 1 {
			t.Error("expected one transfer audit row")
		}
	})

	t.Run("to_amount must match amount for the same currency", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		a := e.account(t, "A", "USD", 10_000)
		b := e.account(t, "B", "USD", 0)
		wantError(t, transfer(e, a, b, map[string]any{"amount": 10.0, "to_amount": 11.0}), "to_amount must be omitted or equal amount")
		wantOK(t, transfer(e, a, b, map[string]any{"amount": 10.0, "to_amount": 10.0}))
		if e.balance(t, b.ID) != 1_000 {
			t.Errorf("destination = %d, want 1000", e.balance(t, b.ID))
		}
	})

	t.Run("both amounts count against the ceiling in base", func(t *testing.T) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.0")
		usd := e.account(t, "USD cash", "USD", 100_000_000)
		myr := e.account(t, "MYR cash", "MYR", 0)
		// amount is USD 100 (fine) but to_amount MYR 50,000 = USD 12,500 is over USD 10,000.
		wantError(t, transfer(e, usd, myr, map[string]any{"amount": 100.0, "to_amount": 50_000.0}), "to_amount of MYR 50,000.00")
		if e.txnCount(t) != 0 {
			t.Error("a refused transfer must not be created")
		}
	})
}

func TestRecordRepaymentToolCrossCurrency(t *testing.T) {
	const scope = "write:transactions"
	setup := func(t *testing.T) (*writeEnv, *models.Receivable, *models.Account, *models.Account) {
		e := setupWriteEnv(t, scope)
		e.recordRate(t, "USD", "MYR", "4.1")
		usd := e.account(t, "USD cash", "USD", 10_000)
		myr := e.account(t, "MYR cash", "MYR", 0)
		expense, err := e.s.services.Receivables.CreateSharedExpense(e.user.ID, services.CreateSharedExpenseInput{
			AccountID: usd.ID, Amount: 6_000, Description: "Dinner",
			Kind: models.ReceivableKindSplit, Mode: services.ShareModeFull,
			Counterparties: []services.SharedCounterpartyInput{{Name: "Alex"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		var rec models.Receivable
		e.db.Where("transaction_id = ?", expense.ID).First(&rec)
		return e, &rec, usd, myr
	}
	repay := func(e *writeEnv, rec *models.Receivable, acct *models.Account, args map[string]any) *mcpgo.CallToolResult {
		args["date"] = today()
		args["receivable_id"], args["account_id"] = rec.ID, acct.ID
		return e.call(t, e.ctx(scope), "record_repayment", args)
	}

	t.Run("a missing account_amount is refused with the market suggestion", func(t *testing.T) {
		e, rec, _, myr := setup(t)
		res := repay(e, rec, myr, map[string]any{"amount": 10.0})
		wantError(t, res, "account_amount is required")
		wantError(t, res, "MYR 41.00")
		if e.balance(t, myr.ID) != 0 || e.auditCount(t, "MCP_RECORD_REPAYMENT") != 0 {
			t.Error("a refused repayment must change nothing")
		}
	})

	t.Run("with account_amount the receivable and the account are credited in their own currencies", func(t *testing.T) {
		e, rec, _, myr := setup(t)
		text := wantOK(t, repay(e, rec, myr, map[string]any{"amount": 10.0, "account_amount": 41.2}))
		for _, want := range []string{"Recorded repayment of USD 10.00", "Credited to the account: MYR 41.20", "paid USD 10.00", "MYR cash", "balance is now MYR 41.20"} {
			if !strings.Contains(text, want) {
				t.Errorf("output missing %q:\n%s", want, text)
			}
		}
		if e.balance(t, myr.ID) != 4_120 {
			t.Errorf("MYR balance = %d, want 4120", e.balance(t, myr.ID))
		}
		var got models.Receivable
		e.db.First(&got, "id = ?", rec.ID)
		if got.PaidAmount != 1_000 {
			t.Errorf("paid = %d, want 1000", got.PaidAmount)
		}
	})

	t.Run("account_amount must match amount in the same currency", func(t *testing.T) {
		e, rec, usd, _ := setup(t)
		wantError(t, repay(e, rec, usd, map[string]any{"amount": 10.0, "account_amount": 11.0}), "account_amount must be omitted or equal amount")
		wantOK(t, repay(e, rec, usd, map[string]any{"amount": 10.0}))
	})

	t.Run("account_amount counts against the ceiling in base", func(t *testing.T) {
		e, rec, _, myr := setup(t)
		wantError(t, repay(e, rec, myr, map[string]any{"amount": 10.0, "account_amount": 50_000.0}), "account_amount of MYR 50,000.00")
	})
}

func TestWriteToolDescriptionsStateTheAmountUnit(t *testing.T) {
	e := setupWriteEnv(t)
	for _, tc := range []struct{ tool, param, want string }{
		{"create_transaction", "amount", "account's own currency"},
		{"create_transfer", "amount", "source account's currency"},
		{"create_transfer", "to_amount", "destination account's currency"},
		{"update_transaction", "amount", "account currency"},
		{"record_repayment", "amount", "receivable's currency"},
		{"record_repayment", "account_amount", "account's currency"},
		{"write_off_receivable", "amount", "receivable's currency"},
		{"create_budget", "amount", "budget's currency"},
		{"update_budget", "amount", "budget's own currency"},
	} {
		st := e.s.mcp.GetTool(tc.tool)
		if st == nil {
			t.Fatalf("tool %s missing", tc.tool)
		}
		prop, ok := st.Tool.InputSchema.Properties[tc.param].(map[string]any)
		if !ok {
			t.Fatalf("%s has no %s parameter", tc.tool, tc.param)
		}
		if desc, _ := prop["description"].(string); !strings.Contains(desc, tc.want) {
			t.Errorf("%s.%s description %q does not state the unit (%q)", tc.tool, tc.param, desc, tc.want)
		}
	}
}
