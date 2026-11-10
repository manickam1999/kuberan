package services

import (
	"bytes"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/fx"
	"kuberan/internal/models"
	"kuberan/internal/testutil"
)

const (
	ibkrFixture2025 = "../importers/ibkr/testdata/statement_2025.csv"
	ibkrFixture2026 = "../importers/ibkr/testdata/statement_2026.csv"
)

// ibkrFinal are the IBKR positions on 2026-11-02 (the 2026 statement's end).
var ibkrFinal = map[string]float64{
	"CCJ": 30.0511, "CHA": 0, "IBKR": 1.6706, "NVDA": 10.2137, "NVO": 30.2548, "SPCX": 10, "VUAA": 45.2, "VWRA": 37,
	"RBOT": 0, "SOUN": 0,
}

var ibkrSecurities = map[string]string{
	"CCJ": "NYSE", "CHA": "NASDAQ", "IBKR": "NASDAQ", "NVDA": "NASDAQ", "NVO": "NYSE", "SPCX": "NASDAQ",
	"VUAA": "LSE", "VWRA": "LSE", "RBOT": "LSE", "SOUN": "NASDAQ",
}

type importEnv struct {
	db      *gorm.DB
	svc     ImportServicer
	acctSvc AccountServicer
	txSvc   TransactionServicer
	userID  string
	inv     *models.Account
	cash    *models.Account // new USD cash account
	legacy  *models.Account // MYR "IBKR Cash"
	bank    *models.Account
	secs    map[string]*models.Security
	transf  *models.Transaction // MYR 33,000.00 transfer bank -> legacy on 2026-06-03
}

func fixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// newImportEnv seeds the acceptance scenario: a MYR user, an investment account
// holding legacy MYR trades for NVO and CCJ, USD securities for all 10 symbols,
// a MYR legacy "IBKR Cash" account fed by a transfer on 2026-06-03, a new USD
// cash account and USD->MYR rates covering 2025-01-01 to 2026-11-02.
func newImportEnv(t *testing.T, ratesFrom string) *importEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	fxs := NewFXService(db, 24*time.Hour)
	acctSvc := NewAccountService(db, WithFXService(fxs))
	txSvc := NewTransactionService(db, acctSvc, nil, WithTransactionFXService(fxs))
	e := &importEnv{db: db, svc: NewImportService(db, acctSvc, fxs), acctSvc: acctSvc, txSvc: txSvc, secs: map[string]*models.Security{}}
	e.userID = testutil.CreateTestUser(t, db).ID
	e.inv = testutil.CreateTestInvestmentAccount(t, db, e.userID)
	e.cash = testutil.CreateTestCashAccount(t, db, e.userID) // USD, balance 0

	mk := func(name, ccy string, bal int64) *models.Account {
		a := &models.Account{UserID: e.userID, Name: name, Type: models.AccountTypeCash, Balance: bal, Currency: ccy, IsActive: true}
		testutil.AssertNoError(t, db.Create(a).Error)
		return a
	}
	e.bank = mk("Bank", "MYR", 10000000)
	e.legacy = mk("IBKR Cash", "MYR", 0)

	for sym, ex := range ibkrSecurities {
		sec := &models.Security{Symbol: sym, Name: sym + " Inc", AssetType: models.AssetTypeStock, Currency: "USD", Exchange: ex}
		testutil.AssertNoError(t, db.Create(sec).Error)
		e.secs[sym] = sec
	}

	// Legacy MYR trades (no FX columns) for two symbols.
	for _, sym := range []string{"NVO", "CCJ"} {
		inv := &models.Investment{AccountID: e.inv.ID, SecurityID: e.secs[sym].ID, Quantity: 5, CostBasis: 100000, CostBasisBase: 100000}
		testutil.AssertNoError(t, db.Create(inv).Error)
		for i := 0; i < 2; i++ {
			testutil.AssertNoError(t, db.Create(&models.InvestmentTransaction{
				InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: time.Date(2025, 2, 1+i, 0, 0, 0, 0, time.UTC),
				Quantity: 2.5, PricePerUnit: 20000, TotalAmount: 50000,
			}).Error)
		}
	}

	start, err := time.Parse("2006-01-02", ratesFrom)
	testutil.AssertNoError(t, err)
	rate, err := fx.ParseRate("4.5")
	testutil.AssertNoError(t, err)
	var in []FXRateInput
	for d := start; !d.After(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		in = append(in, FXRateInput{Base: "USD", Quote: "MYR", Rate: rate, RecordedAt: d, Source: "test"})
	}
	_, err = fxs.RecordRates(in)
	testutil.AssertNoError(t, err)

	// An older, out-of-window top-up and the transfer the 2026-06-04 deposit must match.
	_, err = txSvc.CreateTransfer(e.userID, e.bank.ID, e.legacy.ID, 1000000, nil, "old top-up", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), TransactionMeta{})
	testutil.AssertNoError(t, err)
	e.transf, err = txSvc.CreateTransfer(e.userID, e.bank.ID, e.legacy.ID, 3300000, nil, "IBKR top-up", time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), TransactionMeta{})
	testutil.AssertNoError(t, err)
	return e
}

func (e *importEnv) params(apply bool) IBKRImportParams {
	return IBKRImportParams{
		InvestmentAccountID: e.inv.ID, CashAccountID: e.cash.ID, LegacyCashAccountID: e.legacy.ID,
		CashStartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Apply: apply,
	}
}

func (e *importEnv) files(t *testing.T, mutate func(name string, b []byte) []byte) []io.Reader {
	t.Helper()
	var out []io.Reader
	// Passed out of order on purpose: the import sorts by period.
	for _, p := range []string{ibkrFixture2026, ibkrFixture2025} {
		b := fixtureBytes(t, p)
		if mutate != nil {
			b = mutate(p, b)
		}
		out = append(out, bytes.NewReader(b))
	}
	return out
}

type importCounts struct {
	tx, invTx, inv int64
	balances       map[string]int64
}

func (e *importEnv) counts(t *testing.T) importCounts {
	t.Helper()
	c := importCounts{balances: map[string]int64{}}
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Count(&c.tx).Error)
	testutil.AssertNoError(t, e.db.Model(&models.InvestmentTransaction{}).Count(&c.invTx).Error)
	testutil.AssertNoError(t, e.db.Model(&models.Investment{}).Count(&c.inv).Error)
	var accts []models.Account
	testutil.AssertNoError(t, e.db.Find(&accts).Error)
	for i := range accts {
		c.balances[accts[i].ID] = accts[i].Balance
	}
	return c
}

func sameCounts(a, b importCounts) bool {
	if a.tx != b.tx || a.invTx != b.invTx || a.inv != b.inv || len(a.balances) != len(b.balances) {
		return false
	}
	for k, v := range a.balances {
		if b.balances[k] != v {
			return false
		}
	}
	return true
}

func (e *importEnv) holdings(t *testing.T) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	for sym, sec := range e.secs {
		var inv models.Investment
		err := e.db.Where("account_id = ? AND security_id = ?", e.inv.ID, sec.ID).First(&inv).Error
		if err == nil {
			out[sym] = inv.Quantity
		}
	}
	return out
}

func balanceOf(t *testing.T, db *gorm.DB, id string) int64 {
	t.Helper()
	var a models.Account
	testutil.AssertNoError(t, db.First(&a, "id = ?", id).Error)
	return a.Balance
}

func hasIssue(r *IBKRImportReport, code string) bool {
	for _, is := range r.Errors {
		if is.Code == code {
			return true
		}
	}
	return false
}

func TestImportIBKR_DryRunWritesNothing(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	before := e.counts(t)

	r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
	testutil.AssertNoError(t, err)

	if r.Applied {
		t.Error("dry run reported applied")
	}
	if after := e.counts(t); !sameCounts(before, after) {
		t.Errorf("dry run changed the database: before %+v after %+v", before, after)
	}
	if len(r.Errors) != 0 {
		t.Fatalf("errors: %+v", r.Errors)
	}
	if r.Account != "U00000000" || r.BaseCurrency != "USD" || len(r.Periods) != 2 {
		t.Errorf("metadata: %q %q %d periods", r.Account, r.BaseCurrency, len(r.Periods))
	}
	if len(r.Securities) != 10 {
		t.Errorf("securities = %d, want 10", len(r.Securities))
	}
	for _, s := range r.Securities {
		if len(s.Checkpoints) != 2 {
			t.Errorf("%s: %d checkpoints", s.Symbol, len(s.Checkpoints))
		}
		for _, c := range s.Checkpoints {
			if !c.QuantityOK {
				t.Errorf("%s %s: quantity %v, IBKR %v", s.Symbol, c.Date, c.Quantity, c.IBKRQuantity)
			}
		}
		if math.Abs(s.Final.Quantity-ibkrFinal[s.Symbol]) > 1e-4 {
			t.Errorf("%s final %v want %v", s.Symbol, s.Final.Quantity, ibkrFinal[s.Symbol])
		}
		wantInv := s.Symbol == "NVO" || s.Symbol == "CCJ"
		if (s.InvestmentID != nil) != wantInv {
			t.Errorf("%s: investment id present=%v, want %v", s.Symbol, s.InvestmentID != nil, wantInv)
		}
		if wantInv && s.ExistingTransactions != 2 {
			t.Errorf("%s existing = %d, want 2", s.Symbol, s.ExistingTransactions)
		}
	}
	if len(r.Cash.DepositsMatched) != 1 || r.Cash.DepositsMatched[0].TransactionID != e.transf.ID ||
		r.Cash.DepositsMatched[0].Amount != 746152 || r.Cash.DepositsMatched[0].SourceAmount != 3300000 {
		t.Errorf("deposit match: %+v", r.Cash.DepositsMatched)
	}
	if r.Cash.OpeningBalance != 465 {
		t.Errorf("opening = %d, want 465", r.Cash.OpeningBalance)
	}
	t.Logf("dry run cash: %+v warnings=%q", r.Cash, r.Warnings)
}

func TestImportIBKR_ApplyAcceptance(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	legacyBefore := balanceOf(t, e.db, e.legacy.ID)

	r, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if !r.Applied || len(r.Errors) != 0 {
		t.Fatalf("applied=%v errors=%+v", r.Applied, r.Errors)
	}

	// Final quantities equal the 2026-11-02 IBKR positions.
	got := e.holdings(t)
	for sym, want := range ibkrFinal {
		if math.Abs(got[sym]-want) > 1e-4 {
			t.Errorf("%s quantity %v, want %v", sym, got[sym], want)
		}
	}
	t.Logf("final quantities: %v", got)

	// Legacy MYR rows are gone; the holdings carry native amounts and base figures.
	var nvo models.Investment
	testutil.AssertNoError(t, e.db.Where("account_id = ? AND security_id = ?", e.inv.ID, e.secs["NVO"].ID).First(&nvo).Error)
	var rows []models.InvestmentTransaction
	testutil.AssertNoError(t, e.db.Where("investment_id = ?", nvo.ID).Find(&rows).Error)
	for _, row := range rows {
		if row.FXRateToBase == nil || row.BaseAmount == nil {
			t.Fatalf("NVO row %s has no FX columns", row.ID)
		}
		if row.Type == models.InvestmentTransactionBuy && *row.BaseAmount != fxScaled(t, row.TotalAmount, "4.5") {
			t.Errorf("NVO buy base %d for total %d", *row.BaseAmount, row.TotalAmount)
		}
	}
	if nvo.CostBasisBase == 0 || nvo.CostBasis == 0 {
		t.Errorf("NVO cost basis %d / %d", nvo.CostBasis, nvo.CostBasisBase)
	}

	// Cash: the account ends at the ledger, within the reported diff of IBKR's 12055 cents.
	cp := r.Cash.Checkpoints[len(r.Cash.Checkpoints)-1]
	cashBal := balanceOf(t, e.db, e.cash.ID)
	if cp.Date != "2026-11-02" || cp.IBKR != 12055 {
		t.Fatalf("cash checkpoint: %+v", cp)
	}
	if cashBal != cp.Ledger || cashBal+cp.Diff != 12055 {
		t.Errorf("cash balance %d, ledger %d, diff %d, want ledger+diff = 12055", cashBal, cp.Ledger, cp.Diff)
	}
	if rowsInLedger := r.Cash.Legs + r.Cash.Dividends + r.Cash.Fees; absInt64(cp.Diff) > int64(rowsInLedger) {
		t.Errorf("cash diff %d exceeds %d rows", cp.Diff, rowsInLedger)
	}
	t.Logf("cash ending %d (IBKR 12055, diff %d), legs=%d dividends=%d fees=%d", cashBal, cp.Diff, r.Cash.Legs, r.Cash.Dividends, r.Cash.Fees)

	// The legacy transfer now credits the USD account.
	var tr models.Transaction
	testutil.AssertNoError(t, e.db.First(&tr, "id = ?", e.transf.ID).Error)
	if tr.ToAccountID == nil || *tr.ToAccountID != e.cash.ID || tr.ToAmount == nil || *tr.ToAmount != 746152 || tr.Amount != 3300000 {
		t.Errorf("transfer after apply: to=%v to_amount=%v amount=%d", tr.ToAccountID, tr.ToAmount, tr.Amount)
	}
	if got := balanceOf(t, e.db, e.legacy.ID); got != legacyBefore-3300000 {
		t.Errorf("legacy balance %d, want %d", got, legacyBefore-3300000)
	}
	if r.Cash.LegacyAccountBalanceAfter == nil || *r.Cash.LegacyAccountBalanceAfter != legacyBefore-3300000 {
		t.Errorf("legacy balance after = %v", r.Cash.LegacyAccountBalanceAfter)
	}
	if bank := balanceOf(t, e.db, e.bank.ID); bank != 10000000-1000000-3300000 {
		t.Errorf("bank balance %d changed", bank)
	}
}

func fxScaled(t *testing.T, amount int64, rate string) int64 {
	t.Helper()
	r, err := fx.ParseRate(rate)
	testutil.AssertNoError(t, err)
	out, err := fx.Convert(amount, r)
	testutil.AssertNoError(t, err)
	return out
}

func TestImportIBKR_SecondApplyIsNoOp(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	first := e.counts(t)

	r, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if second := e.counts(t); !sameCounts(first, second) {
		t.Errorf("second apply changed state: %+v -> %+v", first, second)
	}
	if len(r.Cash.DepositsMatched) != 1 || len(r.Cash.UnmatchedDeposits) != 0 || r.Cash.DepositsMatched[0].TransactionID != e.transf.ID {
		t.Errorf("deposit after re-import: matched %+v unmatched %+v", r.Cash.DepositsMatched, r.Cash.UnmatchedDeposits)
	}
	for _, s := range r.Securities {
		if int(s.ExistingTransactions) != s.NewTransactions && s.InvestmentID != nil {
			t.Errorf("%s existing %d new %d", s.Symbol, s.ExistingTransactions, s.NewTransactions)
		}
	}
}

func TestImportIBKR_ReplacesExistingOpeningBalance(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	acct, err := e.acctSvc.CreateCashAccount(e.userID, "IBKR Cash (USD)", "", "USD", 100000)
	testutil.AssertNoError(t, err)
	p := e.params(true)
	p.CashAccountID = acct.ID

	r, err := e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
	testutil.AssertNoError(t, err)
	cp := r.Cash.Checkpoints[len(r.Cash.Checkpoints)-1]
	if got := balanceOf(t, e.db, acct.ID); got != cp.Ledger {
		t.Errorf("balance %d, ledger %d: the old opening balance was not replaced", got, cp.Ledger)
	}
	var n int64
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Where("account_id = ? AND type = ?", acct.ID, models.TransactionTypeOpeningBalance).Count(&n).Error)
	if n != 1 {
		t.Errorf("opening balance rows = %d, want 1", n)
	}
}

func TestImportIBKR_UnmatchedDepositAndDeactivate(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	// Move the matching transfer out of the +-5 day window.
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Where("id = ?", e.transf.ID).
		Update("date", time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)).Error)

	p := e.params(true)
	p.DeactivateLegacyCashAccount = true
	r, err := e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
	testutil.AssertNoError(t, err)
	if len(r.Cash.DepositsMatched) != 0 || len(r.Cash.UnmatchedDeposits) != 1 {
		t.Fatalf("matched %+v unmatched %+v", r.Cash.DepositsMatched, r.Cash.UnmatchedDeposits)
	}
	if u := r.Cash.UnmatchedDeposits[0]; u.Date != "2026-06-04" || u.Amount != 746152 || u.Kind != "deposit" {
		t.Errorf("unmatched: %+v", u)
	}
	var legacy models.Account
	testutil.AssertNoError(t, e.db.First(&legacy, "id = ?", e.legacy.ID).Error)
	if legacy.IsActive {
		t.Error("legacy account still active")
	}
	if legacy.Balance != 4300000 || r.Cash.LegacyAccountBalanceAfter == nil || *r.Cash.LegacyAccountBalanceAfter != 4300000 {
		t.Errorf("legacy balance %d / %v", legacy.Balance, r.Cash.LegacyAccountBalanceAfter)
	}
	// Without the deposit the ledger is short by it, which is reported.
	cp := r.Cash.Checkpoints[len(r.Cash.Checkpoints)-1]
	if cp.Diff < 746000 {
		t.Errorf("diff %d should include the unmatched deposit", cp.Diff)
	}
	if got := balanceOf(t, e.db, e.cash.ID); got != cp.Ledger {
		t.Errorf("balance %d != ledger %d", got, cp.Ledger)
	}
	// A re-import with the legacy account now inactive still works.
	_, err = e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
	testutil.AssertNoError(t, err)
}

func TestImportIBKR_Refusals(t *testing.T) {
	t.Run("unknown security", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		testutil.AssertNoError(t, e.db.Delete(e.secs["SPCX"]).Error)
		before := e.counts(t)

		r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
		testutil.AssertNoError(t, err)
		if !hasIssue(r, "unknown_security") {
			t.Fatalf("errors: %+v", r.Errors)
		}
		_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
		var ae *apperrors.AppError
		if !asAppError(err, &ae) || ae.Details["report"] == nil {
			t.Error("IMPORT_INVALID carries no report")
		}
		if after := e.counts(t); !sameCounts(before, after) {
			t.Error("a refused apply wrote something")
		}
	})

	t.Run("currency mismatch", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		testutil.AssertNoError(t, e.db.Model(&models.Security{}).Where("id = ?", e.secs["NVO"].ID).Update("currency", "MYR").Error)
		r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
		testutil.AssertNoError(t, err)
		if !hasIssue(r, "currency_mismatch") {
			t.Fatalf("errors: %+v", r.Errors)
		}
		_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
	})

	t.Run("missing fx rate", func(t *testing.T) {
		e := newImportEnv(t, "2025-06-01") // 2025 trades before June have no rate
		before := e.counts(t)
		r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
		testutil.AssertNoError(t, err)
		if !hasIssue(r, "fx_rate_missing") {
			t.Fatalf("errors: %+v", r.Errors)
		}
		for _, is := range r.Errors {
			if is.Code == "fx_rate_missing" && !strings.Contains(is.Detail, "oracle backfill --fx") {
				t.Errorf("detail does not name the remedy: %q", is.Detail)
			}
		}
		_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
		if after := e.counts(t); !sameCounts(before, after) {
			t.Error("a refused apply wrote something")
		}
	})

	t.Run("quantity checkpoint failure", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		mutate := func(name string, b []byte) []byte {
			if name != ibkrFixture2026 {
				return b
			}
			out := strings.Replace(string(b), "Open Positions,Data,Summary,Stocks,USD,VWRA,37,", "Open Positions,Data,Summary,Stocks,USD,VWRA,36,", 1)
			if out == string(b) {
				t.Fatal("fixture line not found")
			}
			return []byte(out)
		}
		r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, mutate))
		testutil.AssertNoError(t, err)
		bad := 0
		for _, s := range r.Securities {
			for _, c := range s.Checkpoints {
				if !c.QuantityOK {
					bad++
					if s.Symbol != "VWRA" || c.Date != "2026-11-02" {
						t.Errorf("unexpected failure %s %s", s.Symbol, c.Date)
					}
				}
			}
		}
		if bad != 1 || len(r.Errors) != 0 {
			t.Fatalf("failed checkpoints = %d, errors = %+v", bad, r.Errors)
		}
		before := e.counts(t)
		_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, mutate))
		testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
		if after := e.counts(t); !sameCounts(before, after) {
			t.Error("a refused apply wrote something")
		}
	})

	t.Run("no opening statement", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		p := e.params(false)
		p.CashStartDate = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		r, err := e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
		testutil.AssertNoError(t, err)
		if !hasIssue(r, "opening_balance_missing") {
			t.Fatalf("errors: %+v", r.Errors)
		}
	})

	t.Run("account validation", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		p := e.params(false)
		p.CashAccountID = e.legacy.ID // MYR, not the statement currency
		_, err := e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)

		p = e.params(false)
		p.InvestmentAccountID = e.cash.ID
		p.CashAccountID = e.legacy.ID
		_, err = e.svc.ImportIBKR(e.userID, p, e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)

		other := testutil.CreateTestUser(t, e.db)
		_, err = e.svc.ImportIBKR(other.ID, e.params(false), e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrAccountNotFound.Code)

		_, err = e.svc.ImportIBKR(e.userID, e.params(false), nil)
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
		_, err = e.svc.ImportIBKR(e.userID, e.params(false), []io.Reader{strings.NewReader("not,a,statement\n")})
		testutil.AssertAppError(t, err, apperrors.ErrInvalidInput.Code)
	})

	t.Run("import already running", func(t *testing.T) {
		e := newImportEnv(t, "2025-01-01")
		mu := &sync.Mutex{}
		mu.Lock()
		importLocks.Store(e.userID, mu)
		t.Cleanup(func() { importLocks.Delete(e.userID) })
		_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
		testutil.AssertAppError(t, err, apperrors.ErrImportInProgress.Code)
		// A dry run is not blocked.
		_, err = e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
		testutil.AssertNoError(t, err)
	})
}

func asAppError(err error, target **apperrors.AppError) bool {
	ae, ok := err.(*apperrors.AppError)
	if ok {
		*target = ae
	}
	return ok
}

func TestImportIBKR_SingleStatementNeedsOpeningStatement(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	// Only the 2026 statement: cash starts 2026-01-01 but no statement ends 2025-12-31.
	r, err := e.svc.ImportIBKR(e.userID, e.params(false), []io.Reader{bytes.NewReader(fixtureBytes(t, ibkrFixture2026))})
	testutil.AssertNoError(t, err)
	if !hasIssue(r, "opening_balance_missing") {
		t.Errorf("errors: %+v", r.Errors)
	}
}

func TestImportIBKR_ReimportIgnoresEditedDescriptions(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	first := e.counts(t)

	// A user edits the description of a purchase leg and of a dividend leg.
	for _, like := range []string{importBuyDesc + "%", importDivDesc + "%"} {
		res := e.db.Model(&models.Transaction{}).Where("description LIKE ?", like).Update("description", "my own note")
		testutil.AssertNoError(t, res.Error)
		if res.RowsAffected == 0 {
			t.Fatalf("no rows matched %q", like)
		}
	}
	_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if second := e.counts(t); !sameCounts(first, second) {
		t.Errorf("re-import after description edits changed state: %+v -> %+v", first, second)
	}
}

func TestImportIBKR_UnreadableRefDirectionRefusesApply(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	_, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	var tr models.Transaction
	testutil.AssertNoError(t, e.db.Where("account_id = ? AND import_ref LIKE ?", e.cash.ID, "%:dr").First(&tr).Error)
	testutil.AssertNoError(t, e.db.Model(&models.Transaction{}).Where("id = ?", tr.ID).Update("import_ref", "ibkr:U00000000:garbled").Error)
	before := e.counts(t)

	r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if !hasIssue(r, "import_ref_invalid") {
		t.Fatalf("errors: %+v", r.Errors)
	}
	_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
	if after := e.counts(t); !sameCounts(before, after) {
		t.Error("a refused apply wrote something")
	}
}

func TestImportIBKR_TradesAfterStatementAreKept(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	inv := &models.Investment{AccountID: e.inv.ID, SecurityID: e.secs["VWRA"].ID, Quantity: 1, CostBasis: 5000, CostBasisBase: 5000}
	testutil.AssertNoError(t, e.db.Create(inv).Error)
	late := &models.InvestmentTransaction{
		InvestmentID: inv.ID, Type: models.InvestmentTransactionBuy, Date: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
		Quantity: 1, PricePerUnit: 5000, TotalAmount: 5000,
	}
	testutil.AssertNoError(t, e.db.Create(late).Error)
	before := e.counts(t)

	r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
	testutil.AssertNoError(t, err)
	found := false
	for _, is := range r.Errors {
		if is.Code == "trades_after_statement" && strings.Contains(is.Detail, late.ID) && strings.Contains(is.Detail, "VWRA") &&
			strings.Contains(is.Detail, "2026-11-03") && strings.Contains(is.Detail, "buy") {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors: %+v", r.Errors)
	}
	_, err = e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertAppError(t, err, apperrors.ErrImportInvalid.Code)
	if after := e.counts(t); !sameCounts(before, after) {
		t.Error("a refused apply wrote something")
	}
	var row models.InvestmentTransaction
	testutil.AssertNoError(t, e.db.First(&row, "id = ?", late.ID).Error)
	if row.TotalAmount != 5000 || row.DeletedAt.Valid {
		t.Errorf("post-coverage row was touched: %+v", row)
	}
}

// resetTransfers replaces the env's legacy transfers with the given MYR amounts
// and dates (bank -> legacy), and sets the USD/MYR market rate on 2026-06-04.
func (e *importEnv) resetTransfers(t *testing.T, market string, specs map[time.Time]int64) map[time.Time]string {
	t.Helper()
	testutil.AssertNoError(t, e.db.Where("to_account_id = ?", e.legacy.ID).Delete(&models.Transaction{}).Error)
	testutil.AssertNoError(t, e.db.Model(&models.Account{}).Where("id = ?", e.bank.ID).Update("balance", 1000000000).Error)
	rate, err := fx.ParseRate(market)
	testutil.AssertNoError(t, err)
	_, err = NewFXService(e.db, 24*time.Hour).RecordRates([]FXRateInput{
		{Base: "USD", Quote: "MYR", Rate: rate, RecordedAt: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC), Source: "test"},
	})
	testutil.AssertNoError(t, err)
	ids := map[time.Time]string{}
	for d, amount := range specs {
		tr, err := e.txSvc.CreateTransfer(e.userID, e.bank.ID, e.legacy.ID, amount, nil, "top-up", d, TransactionMeta{})
		testutil.AssertNoError(t, err)
		ids[d] = tr.ID
	}
	return ids
}

func TestImportIBKR_DepositMatchPrefersPlausibleRate(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	near := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	odd := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	best := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	ids := e.resetTransfers(t, "4.08", map[time.Time]int64{near: 2980506, odd: 2500000, best: 3045993})

	r, err := e.svc.ImportIBKR(e.userID, e.params(true), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if len(r.Cash.DepositsMatched) != 1 || r.Cash.DepositsMatched[0].TransactionID != ids[best] {
		t.Fatalf("matched %+v, want the 2026-06-05 transfer %s", r.Cash.DepositsMatched, ids[best])
	}
	var tr models.Transaction
	testutil.AssertNoError(t, e.db.First(&tr, "id = ?", ids[best]).Error)
	if tr.ToAccountID == nil || *tr.ToAccountID != e.cash.ID || tr.ToAmount == nil || *tr.ToAmount != 746152 {
		t.Errorf("transfer not re-pointed: %+v", tr)
	}
	// The other two stay on the legacy account.
	for _, d := range []time.Time{near, odd} {
		var other models.Transaction
		testutil.AssertNoError(t, e.db.First(&other, "id = ?", ids[d]).Error)
		if other.ToAccountID == nil || *other.ToAccountID != e.legacy.ID {
			t.Errorf("transfer %s was moved", ids[d])
		}
	}
}

func TestImportIBKR_DepositWithNoPlausibleCandidateIsUnmatched(t *testing.T) {
	e := newImportEnv(t, "2025-01-01")
	a := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	e.resetTransfers(t, "4.08", map[time.Time]int64{a: 2500000, b: 3300000}) // 3.35 and 4.42 vs 4.08

	r, err := e.svc.ImportIBKR(e.userID, e.params(false), e.files(t, nil))
	testutil.AssertNoError(t, err)
	if len(r.Cash.DepositsMatched) != 0 || len(r.Cash.UnmatchedDeposits) != 1 {
		t.Fatalf("matched %+v unmatched %+v", r.Cash.DepositsMatched, r.Cash.UnmatchedDeposits)
	}
	c := r.Cash.UnmatchedDeposits[0].Candidates
	if len(c) != 2 {
		t.Fatalf("candidates: %+v", c)
	}
	for _, x := range c {
		if math.Abs(x.DeviationPct) <= 3 || x.Amount == 0 || x.Date == "" || x.TransactionID == "" {
			t.Errorf("candidate: %+v", x)
		}
	}
}
