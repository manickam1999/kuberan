package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// Plan 024, phase 3a: native trade amounts, base-currency cost basis and
// cross-currency funding for a MYR-base user.

var nativeDay = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type nativeFixture struct {
	db      *gorm.DB
	fx      FXServicer
	svc     InvestmentServicer
	user    *models.User // base MYR
	invAcct *models.Account
	usd     *models.Security // USD security
	myr     *models.Security // MYR security
	cashUSD *models.Account
	cashMYR *models.Account
}

// newNativeFixture builds the fixture; withRate seeds USD/MYR = 4.4712.
func newNativeFixture(t *testing.T, withRate bool) *nativeFixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })

	fxs := NewFXService(db, testFXStaleness)
	user := testutil.CreateTestUser(t, db)
	usd := testutil.CreateTestSecurity(t, db)
	testutil.AssertNoError(t, db.Model(usd).Update("currency", "USD").Error)
	cashMYR := testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10_000_000)
	testutil.AssertNoError(t, db.Model(cashMYR).Update("currency", "MYR").Error)
	f := &nativeFixture{
		db:      db,
		fx:      fxs,
		svc:     NewInvestmentService(db, NewAccountService(db, WithFXService(fxs)), fxs),
		user:    user,
		invAcct: testutil.CreateTestInvestmentAccount(t, db, user.ID),
		usd:     usd,
		myr:     testutil.CreateTestSecurity(t, db),
		cashUSD: testutil.CreateTestCashAccountWithBalance(t, db, user.ID, 10_000_000), // USD
		cashMYR: cashMYR,
	}
	if withRate {
		seedRate(t, fxs, "USD", "MYR", "4.4712", nativeDay.Add(-time.Hour))
	}
	return f
}

// holding creates an empty position in sec.
func (f *nativeFixture) holding(t *testing.T, sec *models.Security) *models.Investment {
	t.Helper()
	inv := &models.Investment{AccountID: f.invAcct.ID, SecurityID: sec.ID}
	testutil.AssertNoError(t, f.db.Create(inv).Error)
	return inv
}

func (f *nativeFixture) reload(t *testing.T, inv *models.Investment) *models.Investment {
	t.Helper()
	var out models.Investment
	testutil.AssertNoError(t, f.db.First(&out, "id = ?", inv.ID).Error)
	return &out
}

func (f *nativeFixture) balance(t *testing.T, a *models.Account) int64 {
	t.Helper()
	return accountBalance(t, f.db, a.ID)
}

func wantRate(t *testing.T, tx *models.InvestmentTransaction, rate string, base int64) {
	t.Helper()
	if tx.FXRateToBase == nil || tx.FXRateToBase.String() != rate {
		t.Errorf("fx_rate_to_base = %v, want %s", tx.FXRateToBase, rate)
	}
	if tx.BaseAmount == nil || *tx.BaseAmount != base {
		t.Errorf("base_amount = %v, want %d", tx.BaseAmount, base)
	}
}

func appDetails(t *testing.T, err error, code string) map[string]any {
	t.Helper()
	testutil.AssertAppError(t, err, code)
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("not an AppError: %v", err)
	}
	return ae.Details
}

func TestNativeTrades_BaseCurrencySecurity(t *testing.T) {
	f := newNativeFixture(t, false) // identity needs no rate
	inv := f.holding(t, f.myr)

	buy, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 10, PricePerUnit: 5000, Fee: 100, FundingAccountID: f.cashMYR.ID})
	testutil.AssertNoError(t, err)
	wantRate(t, buy, "1", 50100)
	if buy.Currency != "MYR" {
		t.Errorf("currency = %q", buy.Currency)
	}
	if got := f.balance(t, f.cashMYR); got != 10_000_000-50100 {
		t.Errorf("cash after buy = %d", got)
	}

	sell, err := f.svc.RecordSell(f.user.ID, inv.ID, SellParams{Date: nativeDay.Add(24 * time.Hour), Quantity: 4, PricePerUnit: 6000, Fee: 50, DepositAccountID: f.cashMYR.ID})
	testutil.AssertNoError(t, err)
	// proceeds 23950, removed round(50100*0.4) = 20040
	wantRate(t, sell, "1", 23950)
	if sell.RealizedGainLoss != 3910 || sell.RealizedGainLossBase != 3910 {
		t.Errorf("realized = %d / %d, want 3910 both", sell.RealizedGainLoss, sell.RealizedGainLossBase)
	}
	if got := f.balance(t, f.cashMYR); got != 10_000_000-50100+23950 {
		t.Errorf("cash after sell = %d", got)
	}

	div, err := f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay.Add(48 * time.Hour), Amount: 1000, Fee: 100, DepositAccountID: f.cashMYR.ID})
	testutil.AssertNoError(t, err)
	wantRate(t, div, "1", 1000)
	if got := f.balance(t, f.cashMYR); got != 10_000_000-50100+23950+900 {
		t.Errorf("cash after dividend = %d (net 900 expected)", got)
	}

	h := f.reload(t, inv)
	if h.Quantity != 6 || h.CostBasis != 30060 || h.CostBasisBase != 30060 || h.RealizedGainLoss != 3910 || h.RealizedGainLossBase != 3910 {
		t.Errorf("holding = %+v", h)
	}
}

func TestNativeTrades_USDSecurityFundedFromUSDAccountUsesMarketRate(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)

	buy, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 10, PricePerUnit: 17000, FundingAccountID: f.cashUSD.ID})
	testutil.AssertNoError(t, err)
	wantRate(t, buy, "4.4712", 760104) // 170000 USD cents * 4.4712
	if buy.Currency != "USD" || buy.TotalAmount != 170000 {
		t.Errorf("tx = %+v", buy)
	}
	if got := f.balance(t, f.cashUSD); got != 10_000_000-170000 {
		t.Errorf("USD cash = %d", got)
	}
	h := f.reload(t, inv)
	if h.CostBasis != 170000 || h.CostBasisBase != 760104 {
		t.Errorf("holding cost = %d / base %d", h.CostBasis, h.CostBasisBase)
	}

	// A stated funding_amount equal to the total is accepted; a different one is not.
	_, err = f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100, FundingAccountID: f.cashUSD.ID, FundingAmount: 100})
	testutil.AssertNoError(t, err)
	_, err = f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100, FundingAccountID: f.cashUSD.ID, FundingAmount: 101})
	testutil.AssertAppError(t, err, "INVALID_INPUT")
	_, err = f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100, FundingAmount: 100})
	testutil.AssertAppError(t, err, "INVALID_INPUT") // an amount with no account
}

func TestNativeTrades_USDSecurityFundedFromMYRAccount(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)
	params := BuyParams{Date: nativeDay, Quantity: 3, PricePerUnit: 3333, Fee: 1, FundingAccountID: f.cashMYR.ID} // total 10000

	t.Run("funding_amount_required_with_suggestion", func(t *testing.T) {
		_, err := f.svc.RecordBuy(f.user.ID, inv.ID, params)
		d := appDetails(t, err, "FX_AMOUNT_REQUIRED")
		if d["currency"] != "MYR" || d["field"] != "funding_amount" || d["suggested_amount"] != int64(44712) || d["rate"] != "4.4712" {
			t.Errorf("details = %v", d)
		}
		if at, _ := d["rate_recorded_at"].(string); at != nativeDay.Add(-time.Hour).Format(time.RFC3339) {
			t.Errorf("rate_recorded_at = %v", d["rate_recorded_at"])
		}
		if got := f.balance(t, f.cashMYR); got != 10_000_000 {
			t.Errorf("a rejected buy must not move money: %d", got)
		}
		if h := f.reload(t, inv); h.Quantity != 0 {
			t.Errorf("a rejected buy must not change the holding: %+v", h)
		}
	})

	t.Run("implied_rate_is_used_and_account_debited_exactly", func(t *testing.T) {
		p := params
		p.FundingAmount = 44800
		buy, err := f.svc.RecordBuy(f.user.ID, inv.ID, p)
		testutil.AssertNoError(t, err)
		wantRate(t, buy, "4.48", 44800) // beats the market rate 4.4712
		if got := f.balance(t, f.cashMYR); got != 10_000_000-44800 {
			t.Errorf("MYR cash = %d, want debited exactly 44800", got)
		}
		var cash models.Transaction
		testutil.AssertNoError(t, f.db.Where("account_id = ? AND type = ?", f.cashMYR.ID, models.TransactionTypeInvestment).First(&cash).Error)
		if cash.Amount != 44800 {
			t.Errorf("cash transaction amount = %d", cash.Amount)
		}
		if h := f.reload(t, inv); h.CostBasis != 10000 || h.CostBasisBase != 44800 {
			t.Errorf("holding cost = %d / base %d", h.CostBasis, h.CostBasisBase)
		}
	})

	t.Run("explicit_fx_rate_beats_both", func(t *testing.T) {
		p := params
		p.FundingAmount = 44800
		p.FXRate = "4.5"
		buy, err := f.svc.RecordBuy(f.user.ID, inv.ID, p)
		testutil.AssertNoError(t, err)
		wantRate(t, buy, "4.5", 45000)
		if buy.TotalAmount != 10000 {
			t.Errorf("native total = %d", buy.TotalAmount)
		}
	})

	t.Run("invalid_fx_rate", func(t *testing.T) {
		for _, bad := range []string{"abc", "0", "-1", "1.00000000001", "1e3"} {
			p := params
			p.FundingAmount = 44800
			p.FXRate = bad
			_, err := f.svc.RecordBuy(f.user.ID, inv.ID, p)
			testutil.AssertAppError(t, err, "INVALID_INPUT")
		}
	})
}

func TestNativeTrades_NoMarketRateGivesOnlyCurrencyAndField(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.usd)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 1000, FundingAccountID: f.cashMYR.ID})
	d := appDetails(t, err, "FX_AMOUNT_REQUIRED")
	if len(d) != 2 || d["currency"] != "MYR" || d["field"] != "funding_amount" {
		t.Errorf("details = %v, want only currency and field", d)
	}
}

func TestNativeTrades_FXRateUnavailable(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.usd)
	testutil.AssertNoError(t, f.db.Model(inv).Update("quantity", 5).Error) // so the sell is not rejected for size first

	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 1000, FundingAccountID: f.cashUSD.ID})
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
	if !strings.Contains(err.Error(), "fx_rate") {
		t.Errorf("message should ask for fx_rate: %v", err)
	}
	_, err = f.svc.RecordSell(f.user.ID, inv.ID, SellParams{Date: nativeDay, Quantity: 1, PricePerUnit: 1000})
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
	_, err = f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 100})
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
	_, err = f.svc.AddInvestment(f.user.ID, AddInvestmentParams{AccountID: f.invAcct.ID, SecurityID: f.usd.ID, Quantity: 1, PurchasePrice: 1000, Date: &nativeDay})
	testutil.AssertAppError(t, err, "FX_RATE_UNAVAILABLE")
	if got := f.balance(t, f.cashUSD); got != 10_000_000 {
		t.Errorf("nothing may be debited: %d", got)
	}

	// Passing fx_rate succeeds.
	added, err := f.svc.AddInvestment(f.user.ID, AddInvestmentParams{AccountID: f.invAcct.ID, SecurityID: f.usd.ID, Quantity: 2, PurchasePrice: 1000, Fee: 10, Date: &nativeDay, FXRate: "4.2"})
	testutil.AssertNoError(t, err)
	if added.CostBasis != 2010 || added.CostBasisBase != 8442 || added.Currency != "USD" {
		t.Errorf("added = cost %d base %d ccy %q", added.CostBasis, added.CostBasisBase, added.Currency)
	}
	var row models.InvestmentTransaction
	testutil.AssertNoError(t, f.db.Where("investment_id = ?", added.ID).First(&row).Error)
	wantRate(t, &row, "4.2", 8442)
}

func TestNativeTrades_PartialThenFullSellZeroesBaseCost(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)

	b1, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 10, PricePerUnit: 17000}) // market 4.4712
	testutil.AssertNoError(t, err)
	b2, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay.Add(24 * time.Hour), Quantity: 5, PricePerUnit: 18000, FXRate: "4.6"})
	testutil.AssertNoError(t, err)
	// cost 260000 native; base 760104 + 414000 = 1174104.
	if h := f.reload(t, inv); h.CostBasis != 260000 || h.CostBasisBase != 1174104 {
		t.Fatalf("after buys: %+v", h)
	}

	s1, err := f.svc.RecordSell(f.user.ID, inv.ID, SellParams{Date: nativeDay.Add(48 * time.Hour), Quantity: 6, PricePerUnit: 20000, FXRate: "4.5"})
	testutil.AssertNoError(t, err)
	// native: 120000 - round(260000*0.4)=104000 -> 16000; base: 540000 - round(1174104*0.4)=469642 -> 70358.
	if s1.RealizedGainLoss != 16000 || s1.RealizedGainLossBase != 70358 {
		t.Errorf("sell 1 realized = %d / %d", s1.RealizedGainLoss, s1.RealizedGainLossBase)
	}
	if h := f.reload(t, inv); h.CostBasis != 156000 || h.CostBasisBase != 704462 {
		t.Errorf("after partial sell: %+v", h)
	}

	s2, err := f.svc.RecordSell(f.user.ID, inv.ID, SellParams{Date: nativeDay.Add(72 * time.Hour), Quantity: 9, PricePerUnit: 19000, FXRate: "4.4"})
	testutil.AssertNoError(t, err)
	h := f.reload(t, inv)
	if h.Quantity != 0 || h.CostBasis != 0 || h.CostBasisBase != 0 {
		t.Fatalf("a full sell must leave no residue: %+v", h)
	}
	// realized base = base proceeds - base buys, exactly.
	wantBase := *s1.BaseAmount + *s2.BaseAmount - *b1.BaseAmount - *b2.BaseAmount
	if got := s1.RealizedGainLossBase + s2.RealizedGainLossBase; got != wantBase || h.RealizedGainLossBase != wantBase {
		t.Errorf("realized base = %d (holding %d), want %d", got, h.RealizedGainLossBase, wantBase)
	}
	if want := s1.TotalAmount + s2.TotalAmount - b1.TotalAmount - b2.TotalAmount; h.RealizedGainLoss != want {
		t.Errorf("realized native = %d, want %d", h.RealizedGainLoss, want)
	}

	// The portfolio reports the realized base figure for the closed position.
	p, err := f.svc.GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)
	if p.TotalRealizedGainLoss != wantBase || p.TotalCostBasis != 0 {
		t.Errorf("portfolio realized %d cost %d, want %d / 0", p.TotalRealizedGainLoss, p.TotalCostBasis, wantBase)
	}
}

func TestNativeTrades_Dividends(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 10, PricePerUnit: 1000})
	testutil.AssertNoError(t, err)
	before := f.reload(t, inv)

	t.Run("USD_account_gets_net", func(t *testing.T) {
		div, err := f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 1000, Fee: 150, DividendType: "Cash", DepositAccountID: f.cashUSD.ID})
		testutil.AssertNoError(t, err)
		wantRate(t, div, "4.4712", 4471) // the gross, 1000 * 4.4712 = 4471.2
		if div.TotalAmount != 1000 || div.Fee != 150 || div.Currency != "USD" {
			t.Errorf("tx = %+v", div)
		}
		if got := f.balance(t, f.cashUSD); got != 10_000_000+850 {
			t.Errorf("USD cash = %d, want net 850 credited", got)
		}
	})

	t.Run("MYR_account_requires_deposit_amount", func(t *testing.T) {
		p := DividendParams{Date: nativeDay, Amount: 1000, Fee: 150, DepositAccountID: f.cashMYR.ID}
		_, err := f.svc.RecordDividend(f.user.ID, inv.ID, p)
		d := appDetails(t, err, "FX_AMOUNT_REQUIRED")
		if d["currency"] != "MYR" || d["field"] != "deposit_amount" || d["suggested_amount"] != int64(3801) { // net 850 at 4.4712 is 3800.52
			t.Errorf("details = %v", d)
		}

		p.DepositAmount = 3790
		div, err := f.svc.RecordDividend(f.user.ID, inv.ID, p)
		testutil.AssertNoError(t, err)
		if got := f.balance(t, f.cashMYR); got != 10_000_000+3790 {
			t.Errorf("MYR cash = %d, want exactly 3790 credited", got)
		}
		implied, err := fx.RateFromAmounts(850, 3790) // the cash leg settles the net
		testutil.AssertNoError(t, err)
		gross, err := fx.Convert(1000, implied)
		testutil.AssertNoError(t, err)
		wantRate(t, div, implied.String(), gross)
	})

	t.Run("validation", func(t *testing.T) {
		_, err := f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 100, Fee: 101})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
		_, err = f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 100, Fee: -1})
		testutil.AssertAppError(t, err, "INVALID_INPUT")
		_, err = f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 100, Fee: 100, DepositAccountID: f.cashUSD.ID})
		testutil.AssertAppError(t, err, "INVALID_INPUT") // nothing net to deposit
	})

	t.Run("no_deposit_moves_no_cash", func(t *testing.T) {
		usd, myr := f.balance(t, f.cashUSD), f.balance(t, f.cashMYR)
		_, err := f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: nativeDay, Amount: 500})
		testutil.AssertNoError(t, err)
		if f.balance(t, f.cashUSD) != usd || f.balance(t, f.cashMYR) != myr {
			t.Error("a dividend without a deposit account must not move cash")
		}
	})

	after := f.reload(t, inv)
	if after.CostBasis != before.CostBasis || after.CostBasisBase != before.CostBasisBase ||
		after.RealizedGainLoss != before.RealizedGainLoss || after.RealizedGainLossBase != before.RealizedGainLossBase {
		t.Errorf("dividends must not touch cost basis or realized P&L: %+v -> %+v", before, after)
	}
}

func TestNativeTrades_SellDepositIntoMYRAccount(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay.Add(-24 * time.Hour), Quantity: 10, PricePerUnit: 10000, FXRate: "4"})
	testutil.AssertNoError(t, err)

	p := SellParams{Date: nativeDay, Quantity: 5, PricePerUnit: 12000, Fee: 100, DepositAccountID: f.cashMYR.ID} // proceeds 59900
	_, err = f.svc.RecordSell(f.user.ID, inv.ID, p)
	d := appDetails(t, err, "FX_AMOUNT_REQUIRED")
	wantSuggestion, cerr := fx.Convert(59900, mustRate(t, "4.4712"))
	testutil.AssertNoError(t, cerr)
	if d["field"] != "deposit_amount" || d["currency"] != "MYR" || d["suggested_amount"] != wantSuggestion {
		t.Errorf("details = %v, want suggested_amount %d", d, wantSuggestion)
	}

	p.DepositAmount = 268000
	sell, err := f.svc.RecordSell(f.user.ID, inv.ID, p)
	testutil.AssertNoError(t, err)
	r, _ := fx.RateFromAmounts(59900, 268000)
	wantRate(t, sell, r.String(), 268000)
	if got := f.balance(t, f.cashMYR); got != 10_000_000+268000 {
		t.Errorf("MYR cash = %d", got)
	}
	// Realized: native proceeds 59900 less cost 50000 gives 9900; base 268000 less 200000 gives 68000.
	if sell.RealizedGainLoss != 9900 || sell.RealizedGainLossBase != 68000 {
		t.Errorf("realized = %d / %d", sell.RealizedGainLoss, sell.RealizedGainLossBase)
	}
}

func TestGetPortfolio_PriceFXSplitInvariant(t *testing.T) {
	f := newNativeFixture(t, true)
	usdInv := f.holding(t, f.usd)
	myrInv := f.holding(t, f.myr)
	usdInv2 := f.holding(t, f.usd)

	// USD lot bought at 4.0, MYR lot at identity, and a lot with a loss.
	_, err := f.svc.RecordBuy(f.user.ID, usdInv.ID, BuyParams{Date: nativeDay, Quantity: 10, PricePerUnit: 17000, FXRate: "4"})
	testutil.AssertNoError(t, err)
	_, err = f.svc.RecordBuy(f.user.ID, myrInv.ID, BuyParams{Date: nativeDay, Quantity: 5, PricePerUnit: 1000})
	testutil.AssertNoError(t, err)
	_, err = f.svc.RecordBuy(f.user.ID, usdInv2.ID, BuyParams{Date: nativeDay, Quantity: 7, PricePerUnit: 3333, FXRate: "4.7"})
	testutil.AssertNoError(t, err)

	now := time.Now()
	usdPrice := testutil.CreateTestSecurityPrice(t, f.db, f.usd.ID, 20000, now.Add(-time.Hour))
	testutil.AssertNoError(t, f.db.Model(usdPrice).Update("currency", "USD").Error)
	testutil.CreateTestSecurityPrice(t, f.db, f.myr.ID, 1200, now.Add(-time.Hour))
	seedRate(t, f.fx, "USD", "MYR", "4.4712", now.Add(-30*time.Minute))

	p, err := f.svc.GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)

	// usdInv: value 200000 -> 894240; cost 170000 native, 680000 base.
	// usdInv2 shares the USD price: 7 x 20000 = 140000 -> 625968; cost 23331 native, 109657 base (23331*4.7=109655.7).
	// myr: value 6000, cost 5000.
	wantCostBase := int64(680000 + 5000 + 109656)
	if p.TotalCostBasis != wantCostBase {
		t.Errorf("total_cost_basis = %d, want %d", p.TotalCostBasis, wantCostBase)
	}
	wantValue := int64(894240 + 6000 + 625968)
	if p.TotalValue != wantValue {
		t.Errorf("total_value = %d, want %d", p.TotalValue, wantValue)
	}
	gain := p.TotalValue - p.TotalCostBasis
	if p.TotalGainLoss != gain {
		t.Errorf("total_gain_loss = %d, want %d", p.TotalGainLoss, gain)
	}
	// +-1 cent per holding for rounding.
	if diff := p.TotalPriceReturn + p.TotalFXReturn - gain; diff < -3 || diff > 3 {
		t.Errorf("price %d + fx %d != gain %d (diff %d)", p.TotalPriceReturn, p.TotalFXReturn, gain, diff)
	}
	// The FX return is only the USD lots' currency effect: positive for the 4.0 lot, and the 4.7 lot lost.
	nowCost2, cerr := fx.Convert(23331, mustRate(t, "4.4712"))
	testutil.AssertNoError(t, cerr)
	wantFX := int64(760104-680000) + nowCost2 - 109656
	if p.TotalFXReturn != wantFX {
		t.Errorf("total_fx_return = %d, want %d", p.TotalFXReturn, wantFX)
	}
	if len(p.Unconverted) != 0 || p.Currency != "MYR" {
		t.Errorf("unconverted %v currency %s", p.Unconverted, p.Currency)
	}
}

func TestGetPortfolio_NativeCostWithoutRateIsUnconverted(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.usd)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 1000, FXRate: "4"})
	testutil.AssertNoError(t, err)
	p, err := f.svc.GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)
	if p.TotalCostBasis != 0 || p.TotalPriceReturn != 0 || p.TotalFXReturn != 0 || len(p.Unconverted) != 1 {
		t.Errorf("a holding with no rate stays out of the totals: %+v", p)
	}
}

func TestInvestmentResponses_CarryCurrency(t *testing.T) {
	f := newNativeFixture(t, true)
	inv := f.holding(t, f.usd)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 1000})
	testutil.AssertNoError(t, err)

	got, err := f.svc.GetInvestmentByID(f.user.ID, inv.ID)
	testutil.AssertNoError(t, err)
	if got.Currency != "USD" || got.PriceCurrency != "USD" {
		t.Errorf("currency = %q / %q", got.Currency, got.PriceCurrency)
	}
	page, err := f.svc.GetInvestmentTransactions(f.user.ID, inv.ID, pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	if len(page.Data) != 1 || page.Data[0].Currency != "USD" {
		t.Errorf("transactions = %+v", page.Data)
	}
	raw, err := json.Marshal(page.Data[0])
	testutil.AssertNoError(t, err)
	var m map[string]any
	testutil.AssertNoError(t, json.Unmarshal(raw, &m))
	if m["fx_rate_to_base"] != "4.4712" || m["currency"] != "USD" || m["base_amount"] != float64(4471) {
		t.Errorf("json = %s", raw)
	}
}

func TestInvestmentTransaction_NullRateRoundTrips(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.myr)
	row := &models.InvestmentTransaction{InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: nativeDay, Quantity: 1, TotalAmount: 5}
	testutil.AssertNoError(t, f.db.Create(row).Error)
	var got models.InvestmentTransaction
	testutil.AssertNoError(t, f.db.First(&got, "id = ?", row.ID).Error)
	if got.FXRateToBase != nil || got.BaseAmount != nil {
		t.Errorf("legacy row must read back NULL: %v %v", got.FXRateToBase, got.BaseAmount)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `"fx_rate_to_base":null`) {
		t.Errorf("json = %s", raw)
	}
}

func TestMigrationShape_NativeInvestmentColumns(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	for table, cols := range map[string][]string{
		"investment_transactions": {"fx_rate_to_base", "base_amount", "realized_gain_loss_base"},
		"investments":             {"cost_basis_base", "realized_gain_loss_base"},
	} {
		for _, c := range cols {
			if !db.Migrator().HasColumn(table, c) {
				t.Errorf("%s.%s missing after AutoMigrate", table, c)
			}
		}
	}
}

// --- replay ---

type rowSnapshot struct {
	Realized, RealizedBase int64
}

func snapshotInvestment(t *testing.T, db *gorm.DB, id string) (inv models.Investment, rowsByID map[string]rowSnapshot) {
	t.Helper()
	testutil.AssertNoError(t, db.First(&inv, "id = ?", id).Error)
	var rows []models.InvestmentTransaction
	testutil.AssertNoError(t, db.Where("investment_id = ?", id).Find(&rows).Error)
	rowsByID = map[string]rowSnapshot{}
	for i := range rows {
		rowsByID[rows[i].ID] = rowSnapshot{rows[i].RealizedGainLoss, rows[i].RealizedGainLossBase}
	}
	return inv, rowsByID
}

func TestRebuildInvestment_MatchesIncrementalPath(t *testing.T) {
	f := newNativeFixture(t, false)
	rng := rand.New(rand.NewSource(20261108))

	for seq := 0; seq < 200; seq++ {
		inv := f.holding(t, f.usd)
		day := 0
		next := func() time.Time { day++; return nativeDay.AddDate(0, 0, day) }
		rate := func() string { return fmt.Sprintf("%d.%04d", 1+rng.Intn(7), rng.Intn(10000)) }

		for i, n := 0, 3+rng.Intn(10); i < n; i++ {
			pos := f.reload(t, inv).Quantity
			op := rng.Intn(10)
			var err error
			switch {
			case op < 4 || pos <= 0:
				_, err = f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{
					Date: next(), Quantity: float64(1+rng.Intn(5000)) / 100, PricePerUnit: int64(100 + rng.Intn(500000)),
					Fee: int64(rng.Intn(500)), FXRate: rate(),
				})
			case op < 7:
				qty := pos
				if rng.Intn(4) != 0 {
					qty = pos * float64(1+rng.Intn(99)) / 100
				}
				_, err = f.svc.RecordSell(f.user.ID, inv.ID, SellParams{
					Date: next(), Quantity: qty, PricePerUnit: int64(100 + rng.Intn(500000)), Fee: int64(rng.Intn(50)), FXRate: rate(),
				})
			case op < 8:
				_, err = f.svc.RecordSplit(f.user.ID, inv.ID, next(), []float64{2, 3, 0.5, 1.5}[rng.Intn(4)], "")
			default:
				amount := int64(100 + rng.Intn(10000))
				_, err = f.svc.RecordDividend(f.user.ID, inv.ID, DividendParams{Date: next(), Amount: amount, Fee: int64(rng.Intn(100)), FXRate: rate()})
			}
			if err != nil {
				t.Fatalf("sequence %d step %d: %v", seq, i, err)
			}
		}

		wantInv, wantRows := snapshotInvestment(t, f.db, inv.ID)

		// Wipe everything replay must recompute, then rebuild.
		testutil.AssertNoError(t, f.db.Model(&models.Investment{}).Where("id = ?", inv.ID).Updates(map[string]interface{}{
			"quantity": 0, "cost_basis": 0, "cost_basis_base": 0, "realized_gain_loss": 0, "realized_gain_loss_base": 0,
		}).Error)
		testutil.AssertNoError(t, f.db.Model(&models.InvestmentTransaction{}).Where("investment_id = ?", inv.ID).Updates(map[string]interface{}{
			"realized_gain_loss": 0, "realized_gain_loss_base": 0,
		}).Error)
		testutil.AssertNoError(t, f.db.Transaction(func(tx *gorm.DB) error { return rebuildInvestment(tx, inv.ID) }))

		gotInv, gotRows := snapshotInvestment(t, f.db, inv.ID)
		if gotInv.Quantity != wantInv.Quantity || gotInv.CostBasis != wantInv.CostBasis || gotInv.CostBasisBase != wantInv.CostBasisBase ||
			gotInv.RealizedGainLoss != wantInv.RealizedGainLoss || gotInv.RealizedGainLossBase != wantInv.RealizedGainLossBase {
			t.Fatalf("sequence %d: holding differs\nincremental: %+v\nreplay:      %+v", seq, wantInv, gotInv)
		}
		for id, w := range wantRows {
			if gotRows[id] != w {
				t.Fatalf("sequence %d: row %s realized differs: incremental %+v, replay %+v", seq, id, w, gotRows[id])
			}
		}
	}
}

func TestRebuildInvestment_OversoldSequenceFails(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.myr)
	_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 5, PricePerUnit: 1000})
	testutil.AssertNoError(t, err)

	// A sell larger than the position that preceded it (as a bad import would insert).
	bad := &models.InvestmentTransaction{
		InvestmentID: inv.ID, Type: models.InvestmentTransactionSell, Date: nativeDay.Add(time.Hour),
		Quantity: 9, TotalAmount: 9000,
	}
	testutil.AssertNoError(t, f.db.Create(bad).Error)

	before, _ := snapshotInvestment(t, f.db, inv.ID)
	err = f.db.Transaction(func(tx *gorm.DB) error { return rebuildInvestment(tx, inv.ID) })
	testutil.AssertAppError(t, err, "INSUFFICIENT_SHARES")
	if !strings.Contains(err.Error(), bad.ID) {
		t.Errorf("error should name the transaction %s: %v", bad.ID, err)
	}
	after, _ := snapshotInvestment(t, f.db, inv.ID)
	if after.Quantity != before.Quantity || after.CostBasis != before.CostBasis || after.CostBasisBase != before.CostBasisBase {
		t.Errorf("a failed rebuild must change nothing: %+v -> %+v", before, after)
	}

	testutil.AssertAppError(t, f.db.Transaction(func(tx *gorm.DB) error { return rebuildInvestment(tx, "00000000-0000-0000-0000-00000000dead") }), "INVESTMENT_NOT_FOUND")
}

func TestRebuildInvestment_LegacyRowsUseTotalAsBase(t *testing.T) {
	f := newNativeFixture(t, false)
	inv := f.holding(t, f.myr)
	buy := &models.InvestmentTransaction{InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: nativeDay, Quantity: 4, TotalAmount: 4000}
	sell := &models.InvestmentTransaction{InvestmentID: inv.ID, Type: models.InvestmentTransactionSell, Date: nativeDay.Add(time.Hour), Quantity: 1, TotalAmount: 1500}
	testutil.AssertNoError(t, f.db.Create(buy).Error)
	testutil.AssertNoError(t, f.db.Create(sell).Error)
	testutil.AssertNoError(t, f.db.Transaction(func(tx *gorm.DB) error { return rebuildInvestment(tx, inv.ID) }))
	h := f.reload(t, inv)
	if h.Quantity != 3 || h.CostBasis != 3000 || h.CostBasisBase != 3000 || h.RealizedGainLoss != 500 || h.RealizedGainLossBase != 500 {
		t.Errorf("holding = %+v", h)
	}
}

func TestHoldingState_SellWholePositionRemovesAllCost(t *testing.T) {
	st := holdingState{Quantity: 3, CostBasis: 1001, CostBasisBase: 4999}
	res, err := st.apply(tradeEvent{Type: models.InvestmentTransactionSell, Quantity: 3, TotalAmount: 2000, BaseAmount: 9000})
	testutil.AssertNoError(t, err)
	if st.CostBasis != 0 || st.CostBasisBase != 0 || st.Quantity != 0 || res.RealizedGainLoss != 999 || res.RealizedGainLossBase != 4001 {
		t.Errorf("state %+v result %+v", st, res)
	}
	if _, err := st.apply(tradeEvent{Type: models.InvestmentTransactionSell, Quantity: 1}); !errors.Is(err, apperrors.ErrInsufficientShares) {
		t.Errorf("selling from an empty position: %v", err)
	}
}

func TestHoldingState_QuantityDust(t *testing.T) {
	buy := func(qty float64, amount int64) tradeEvent {
		return tradeEvent{Type: models.InvestmentTransactionBuy, Quantity: qty, TotalAmount: amount, BaseAmount: amount}
	}
	sell := func(qty float64, proceeds int64) tradeEvent {
		return tradeEvent{Type: models.InvestmentTransactionSell, Quantity: qty, TotalAmount: proceeds, BaseAmount: proceeds}
	}
	split := func(ratio float64) tradeEvent {
		return tradeEvent{Type: models.InvestmentTransactionSplit, SplitRatio: ratio}
	}

	tests := []struct {
		name         string
		start        holdingState
		events       []tradeEvent
		wantErr      error
		wantQuantity float64
		wantCost     int64
		wantCostBase int64
		wantRealized int64 // sum of RealizedGainLoss over sell events
		wantRealBase int64 // sum of RealizedGainLossBase over sell events
	}{
		{
			name: "fractional round trip closes exactly",
			events: []tradeEvent{
				buy(97, 9700),
				buy(5.9936, 599),
				sell(102, 10500),
				sell(0.9936, 100),
			},
			wantQuantity: 0,
			wantCost:     0,
			wantCostBase: 0,
			wantRealized: 10600 - 10299,
			wantRealBase: 10600 - 10299,
		},
		{
			name: "sell 1e-12 above the position is a full sell",
			events: []tradeEvent{
				buy(3, 1001),
				sell(3+1e-12, 2000),
			},
			wantQuantity: 0,
			wantCost:     0,
			wantCostBase: 0,
			wantRealized: 2000 - 1001,
			wantRealBase: 2000 - 1001,
		},
		{
			name: "sell 1e-6 above the position is rejected",
			events: []tradeEvent{
				buy(3, 1001),
				sell(3+1e-6, 2000),
			},
			wantErr:      apperrors.ErrInsufficientShares,
			wantQuantity: 3,
			wantCost:     1001,
			wantCostBase: 1001,
		},
		{
			name:         "sell from a position of dust is rejected",
			start:        holdingState{Quantity: 5e-10, CostBasis: 1, CostBasisBase: 1},
			events:       []tradeEvent{sell(5e-10, 1)},
			wantErr:      apperrors.ErrInsufficientShares,
			wantQuantity: 5e-10,
			wantCost:     1,
			wantCostBase: 1,
		},
		{
			name:         "split leaving dust normalises to zero",
			start:        holdingState{Quantity: 2e-9, CostBasis: 500, CostBasisBase: 500},
			events:       []tradeEvent{split(0.25)},
			wantQuantity: 0,
			wantCost:     500,
			wantCostBase: 500,
		},
		{
			name:         "split without dust keeps the quantity",
			start:        holdingState{Quantity: 3, CostBasis: 900, CostBasisBase: 900},
			events:       []tradeEvent{split(2)},
			wantQuantity: 6,
			wantCost:     900,
			wantCostBase: 900,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.start
			var realized, realizedBase int64
			var gotErr error
			for _, e := range tc.events {
				res, err := st.apply(e)
				if err != nil {
					gotErr = err
					break
				}
				realized += res.RealizedGainLoss
				realizedBase += res.RealizedGainLossBase
			}
			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("error = %v, want %v", gotErr, tc.wantErr)
			}
			if st.Quantity != tc.wantQuantity {
				t.Errorf("Quantity = %v, want %v", st.Quantity, tc.wantQuantity)
			}
			if st.CostBasis != tc.wantCost || st.CostBasisBase != tc.wantCostBase {
				t.Errorf("cost = %d/%d, want %d/%d", st.CostBasis, st.CostBasisBase, tc.wantCost, tc.wantCostBase)
			}
			if realized != tc.wantRealized || realizedBase != tc.wantRealBase {
				t.Errorf("realized = %d/%d, want %d/%d", realized, realizedBase, tc.wantRealized, tc.wantRealBase)
			}
		})
	}
}

// --- security currency lock ---

func TestUpdateSecurity_CurrencyLockedByNativeEraTrades(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	setup := func(t *testing.T) (SecurityServicer, *nativeFixture) {
		t.Helper()
		f := newNativeFixture(t, true)
		return NewSecurityService(f.db), f
	}

	t.Run("native_trade_blocks_even_with_reset", func(t *testing.T) {
		svc, f := setup(t)
		inv := f.holding(t, f.usd)
		_, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100})
		testutil.AssertNoError(t, err)
		_, err = svc.UpdateSecurity(f.usd.ID, SecurityUpdate{Currency: strPtr("MYR"), ResetPrices: true})
		testutil.AssertAppError(t, err, apperrors.ErrSecurityCurrencyLocked.Code)
		_, err = svc.UpdateSecurity(f.usd.ID, SecurityUpdate{Currency: strPtr("MYR")})
		testutil.AssertAppError(t, err, apperrors.ErrSecurityCurrencyLocked.Code)
		var sec models.Security
		testutil.AssertNoError(t, f.db.First(&sec, "id = ?", f.usd.ID).Error)
		if sec.Currency != "USD" {
			t.Errorf("currency changed to %s", sec.Currency)
		}
	})

	t.Run("legacy_rows_do_not_block", func(t *testing.T) {
		svc, f := setup(t)
		inv := f.holding(t, f.usd)
		legacy := &models.InvestmentTransaction{InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: nativeDay, Quantity: 1, TotalAmount: 100}
		testutil.AssertNoError(t, f.db.Create(legacy).Error) // fx_rate_to_base NULL
		res, err := svc.UpdateSecurity(f.usd.ID, SecurityUpdate{Currency: strPtr("MYR")})
		testutil.AssertNoError(t, err)
		if res.Security.Currency != "MYR" {
			t.Errorf("currency = %s", res.Security.Currency)
		}
	})

	t.Run("deleted_native_trade_does_not_block", func(t *testing.T) {
		svc, f := setup(t)
		inv := f.holding(t, f.usd)
		tx, err := f.svc.RecordBuy(f.user.ID, inv.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100})
		testutil.AssertNoError(t, err)
		testutil.AssertNoError(t, f.db.Delete(&models.InvestmentTransaction{}, "id = ?", tx.ID).Error)
		_, err = svc.UpdateSecurity(f.usd.ID, SecurityUpdate{Currency: strPtr("MYR")})
		testutil.AssertNoError(t, err)
	})

	t.Run("other_securitys_trades_do_not_block", func(t *testing.T) {
		svc, f := setup(t)
		other := f.holding(t, f.myr)
		_, err := f.svc.RecordBuy(f.user.ID, other.ID, BuyParams{Date: nativeDay, Quantity: 1, PricePerUnit: 100})
		testutil.AssertNoError(t, err)
		_, err = svc.UpdateSecurity(f.usd.ID, SecurityUpdate{Currency: strPtr("EUR")})
		testutil.AssertNoError(t, err)
	})
}
