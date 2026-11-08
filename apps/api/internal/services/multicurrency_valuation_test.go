package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// mcFixture is a MYR user with a MYR investment account holding 10 units of a
// USD security priced at 17000 USD cents.
type mcFixture struct {
	db   *gorm.DB
	fx   FXServicer
	user *models.User
	acct *models.Account
	sec  *models.Security
	inv  *models.Investment
}

func newMCFixture(t *testing.T) *mcFixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })

	user := testutil.CreateTestUser(t, db) // base currency MYR
	acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	sec := testutil.CreateTestSecurity(t, db)
	testutil.AssertNoError(t, db.Model(sec).Update("currency", "USD").Error)
	inv := testutil.CreateTestInvestment(t, db, acct.ID, sec.ID) // quantity 10, cost basis 100000
	sp := testutil.CreateTestSecurityPrice(t, db, sec.ID, 17000, time.Now().Add(-2*time.Hour))
	testutil.AssertNoError(t, db.Model(sp).Update("currency", "USD").Error)
	return &mcFixture{db: db, fx: NewFXService(db, testFXStaleness), user: user, acct: acct, sec: sec, inv: inv}
}

func (f *mcFixture) seedUSDMYR(t *testing.T, rate string, at time.Time) {
	t.Helper()
	seedRate(t, f.fx, "USD", "MYR", rate, at)
}

func (f *mcFixture) investments(t *testing.T) InvestmentServicer {
	t.Helper()
	return NewInvestmentService(f.db, NewAccountService(f.db, WithFXService(f.fx)), f.fx)
}

func assertPricing(t *testing.T, inv *models.Investment, price int64, rate string, missing, stale bool) {
	t.Helper()
	if inv.CurrentPrice != price {
		t.Errorf("current_price = %d, want %d", inv.CurrentPrice, price)
	}
	if inv.CurrentPriceNative != 17000 || inv.PriceCurrency != "USD" {
		t.Errorf("native = %d %s, want 17000 USD", inv.CurrentPriceNative, inv.PriceCurrency)
	}
	if inv.FXRate != rate || inv.FXMissing != missing || inv.FXStale != stale {
		t.Errorf("fx = %q missing=%v stale=%v, want %q %v %v", inv.FXRate, inv.FXMissing, inv.FXStale, rate, missing, stale)
	}
}

func TestInvestmentResponses_ConvertToBaseCurrency(t *testing.T) {
	f := newMCFixture(t)
	f.seedUSDMYR(t, "4.4712", time.Now().Add(-time.Hour))
	svc := f.investments(t)
	// 17000 * 4.4712 = 76010.4 -> 76010
	const want = 76010

	t.Run("GetInvestmentByID", func(t *testing.T) {
		inv, err := svc.GetInvestmentByID(f.user.ID, f.inv.ID)
		testutil.AssertNoError(t, err)
		assertPricing(t, inv, want, "4.4712", false, false)
	})
	t.Run("GetAccountInvestments", func(t *testing.T) {
		page, err := svc.GetAccountInvestments(f.user.ID, f.acct.ID, pagination.PageRequest{})
		testutil.AssertNoError(t, err)
		assertPricing(t, &page.Data[0], want, "4.4712", false, false)
	})
	t.Run("GetAllInvestments", func(t *testing.T) {
		page, err := svc.GetAllInvestments(f.user.ID, InvestmentStatusOpen, pagination.PageRequest{})
		testutil.AssertNoError(t, err)
		assertPricing(t, &page.Data[0], want, "4.4712", false, false)
	})
	t.Run("AddInvestment", func(t *testing.T) {
		inv, err := svc.AddInvestment(f.user.ID, f.acct.ID, f.sec.ID, 1, 15000, "", nil, 0, "")
		testutil.AssertNoError(t, err)
		assertPricing(t, inv, want, "4.4712", false, false)
	})
}

func TestInvestmentResponses_MissingRate(t *testing.T) {
	f := newMCFixture(t) // no USD/MYR rate seeded
	svc := f.investments(t)

	inv, err := svc.GetInvestmentByID(f.user.ID, f.inv.ID)
	testutil.AssertNoError(t, err)
	assertPricing(t, inv, 0, "", true, false)

	page, err := svc.GetAllInvestments(f.user.ID, InvestmentStatusOpen, pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	assertPricing(t, &page.Data[0], 0, "", true, false)

	page, err = svc.GetAccountInvestments(f.user.ID, f.acct.ID, pagination.PageRequest{})
	testutil.AssertNoError(t, err)
	assertPricing(t, &page.Data[0], 0, "", true, false)

	added, err := svc.AddInvestment(f.user.ID, f.acct.ID, f.sec.ID, 1, 15000, "", nil, 0, "")
	testutil.AssertNoError(t, err)
	assertPricing(t, added, 0, "", true, false)
}

func TestInvestmentResponses_StaleRate(t *testing.T) {
	f := newMCFixture(t)
	f.seedUSDMYR(t, "4.4712", time.Now().Add(-2*testFXStaleness))
	inv, err := f.investments(t).GetInvestmentByID(f.user.ID, f.inv.ID)
	testutil.AssertNoError(t, err)
	assertPricing(t, inv, 76010, "4.4712", false, true)
}

func TestInvestmentResponses_UsesUsersBaseCurrency(t *testing.T) {
	f := newMCFixture(t)
	testutil.AssertNoError(t, f.db.Model(f.user).Update("base_currency", "USD").Error)
	inv, err := f.investments(t).GetInvestmentByID(f.user.ID, f.inv.ID)
	testutil.AssertNoError(t, err)
	// Same currency as the price: no conversion, no rate.
	assertPricing(t, inv, 17000, "", false, false)
}

func TestInvestmentResponses_SameCurrencyNeedsNoFX(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	user := testutil.CreateTestUser(t, db)
	acct := testutil.CreateTestInvestmentAccount(t, db, user.ID)
	sec := testutil.CreateTestSecurity(t, db) // MYR
	inv := testutil.CreateTestInvestment(t, db, acct.ID, sec.ID)
	testutil.CreateTestSecurityPrice(t, db, sec.ID, 500, time.Now())

	got, err := NewInvestmentService(db, NewAccountService(db), nil).GetInvestmentByID(user.ID, inv.ID)
	testutil.AssertNoError(t, err)
	if got.CurrentPrice != 500 || got.CurrentPriceNative != 500 || got.PriceCurrency != "MYR" || got.FXRate != "" || got.FXMissing {
		t.Errorf("unexpected pricing: %+v", got)
	}
}

func TestGetPortfolio_ConvertsLineValueToBase(t *testing.T) {
	f := newMCFixture(t)
	f.seedUSDMYR(t, "4.4712", time.Now().Add(-time.Hour))
	// A second USD holding (3 x 3333 native cents): the line must be rounded once.
	sec2 := testutil.CreateTestSecurityWithParams(t, f.db, "LINE", "Line", models.AssetTypeStock, "NYSE")
	testutil.AssertNoError(t, f.db.Model(sec2).Update("currency", "USD").Error)
	inv2 := testutil.CreateTestInvestment(t, f.db, f.acct.ID, sec2.ID)
	testutil.AssertNoError(t, f.db.Model(inv2).Updates(map[string]interface{}{"quantity": 3, "cost_basis": 40000}).Error)
	testutil.CreateTestSecurityPrice(t, f.db, sec2.ID, 3333, time.Now().Add(-time.Hour))
	// And a MYR holding that needs no rate.
	sec3 := testutil.CreateTestSecurityWithParams(t, f.db, "MYRS", "Myr", models.AssetTypeStock, "KLSE")
	inv3 := testutil.CreateTestInvestment(t, f.db, f.acct.ID, sec3.ID)
	testutil.AssertNoError(t, f.db.Model(inv3).Updates(map[string]interface{}{"quantity": 2, "cost_basis": 900}).Error)
	testutil.CreateTestSecurityPrice(t, f.db, sec3.ID, 500, time.Now().Add(-time.Hour))

	p, err := f.investments(t).GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)

	// 170000 USD cents -> 760104; 3 x 3333 = 9999 -> 44708; 2 x 500 MYR = 1000.
	wantValue := int64(760104 + 44708 + 1000)
	if p.TotalValue != wantValue {
		t.Errorf("total_value = %d, want %d", p.TotalValue, wantValue)
	}
	if p.TotalCostBasis != 100000+40000+900 || p.TotalGainLoss != wantValue-140900 {
		t.Errorf("cost basis %d gain %d", p.TotalCostBasis, p.TotalGainLoss)
	}
	if p.Currency != "MYR" || p.FXStale || len(p.Unconverted) != 0 || p.Unconverted == nil {
		t.Errorf("currency=%q stale=%v unconverted=%v", p.Currency, p.FXStale, p.Unconverted)
	}
	if ts := p.HoldingsByType[models.AssetTypeStock]; ts.Count != 3 || ts.Value != wantValue {
		t.Errorf("holdings_by_type = %+v", ts)
	}
}

func TestGetPortfolio_MissingRateIsUnconverted(t *testing.T) {
	f := newMCFixture(t) // no rate
	sec3 := testutil.CreateTestSecurityWithParams(t, f.db, "MYRS", "Myr", models.AssetTypeStock, "KLSE")
	inv3 := testutil.CreateTestInvestment(t, f.db, f.acct.ID, sec3.ID)
	testutil.AssertNoError(t, f.db.Model(inv3).Updates(map[string]interface{}{"quantity": 2, "cost_basis": 900}).Error)
	testutil.CreateTestSecurityPrice(t, f.db, sec3.ID, 500, time.Now())

	p, err := f.investments(t).GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)

	if p.TotalValue != 1000 || p.TotalCostBasis != 900 || p.TotalGainLoss != 100 {
		t.Errorf("only the MYR holding counts: value=%d cost=%d gain=%d", p.TotalValue, p.TotalCostBasis, p.TotalGainLoss)
	}
	if len(p.Unconverted) != 1 || p.Unconverted[0].Currency != "USD" || p.Unconverted[0].Value != 170000 {
		t.Errorf("unconverted = %+v, want [{USD 170000}]", p.Unconverted)
	}
}

func TestGetPortfolio_StaleRateFlagged(t *testing.T) {
	f := newMCFixture(t)
	f.seedUSDMYR(t, "4.4712", time.Now().Add(-2*testFXStaleness))
	p, err := f.investments(t).GetPortfolio(f.user.ID)
	testutil.AssertNoError(t, err)
	if !p.FXStale || p.TotalValue != 760104 {
		t.Errorf("stale=%v value=%d", p.FXStale, p.TotalValue)
	}
}

func TestGetPortfolio_NoAccountsHasCurrencyAndEmptyUnconverted(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	user := testutil.CreateTestUser(t, db)
	p, err := NewInvestmentService(db, NewAccountService(db), nil).GetPortfolio(user.ID)
	testutil.AssertNoError(t, err)
	if p.Currency != "MYR" || p.Unconverted == nil {
		t.Errorf("currency=%q unconverted=%v", p.Currency, p.Unconverted)
	}
}

func TestInvestmentAccountBalance_ConvertsToAccountCurrency(t *testing.T) {
	f := newMCFixture(t)
	f.seedUSDMYR(t, "4.4712", time.Now().Add(-time.Hour))
	acctSvc := NewAccountService(f.db, WithFXService(f.fx))

	got, err := acctSvc.GetAccountByID(f.user.ID, f.acct.ID)
	testutil.AssertNoError(t, err)
	if got.Balance != 760104 {
		t.Errorf("balance = %d, want 760104", got.Balance)
	}

	// An account in USD values the same holding without conversion.
	testutil.AssertNoError(t, f.db.Model(f.acct).Update("currency", "USD").Error)
	got, err = NewAccountService(f.db).GetAccountByID(f.user.ID, f.acct.ID) // no FX needed
	testutil.AssertNoError(t, err)
	if got.Balance != 170000 {
		t.Errorf("USD account balance = %d, want 170000", got.Balance)
	}
}

func TestInvestmentAccountBalance_MissingRateContributesZero(t *testing.T) {
	f := newMCFixture(t) // no rate
	sec3 := testutil.CreateTestSecurityWithParams(t, f.db, "MYRS", "Myr", models.AssetTypeStock, "KLSE")
	inv3 := testutil.CreateTestInvestment(t, f.db, f.acct.ID, sec3.ID)
	testutil.AssertNoError(t, f.db.Model(inv3).Update("quantity", 2).Error)
	testutil.CreateTestSecurityPrice(t, f.db, sec3.ID, 500, time.Now())

	got, err := NewAccountService(f.db, WithFXService(f.fx)).GetAccountByID(f.user.ID, f.acct.ID)
	testutil.AssertNoError(t, err)
	if got.Balance != 1000 {
		t.Errorf("balance = %d, want 1000 (USD holding contributes 0)", got.Balance)
	}
}

func snapshotOf(t *testing.T, db *gorm.DB, userID string, at time.Time) models.PortfolioSnapshot {
	t.Helper()
	var s models.PortfolioSnapshot
	testutil.AssertNoError(t, db.Where("user_id = ? AND recorded_at = ?", userID, at).First(&s).Error)
	return s
}

func TestSnapshots_ConvertAtRecordedAt(t *testing.T) {
	f := newMCFixture(t)
	t0 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	f.seedUSDMYR(t, "3", t0)
	f.seedUSDMYR(t, "5", t0.AddDate(0, 0, 10))
	svc := NewPortfolioSnapshotService(f.db, f.fx)

	past := t0.AddDate(0, 0, 5)
	_, err := svc.ComputeAndRecordSnapshots(past)
	testutil.AssertNoError(t, err)
	s := snapshotOf(t, f.db, f.user.ID, past)
	if s.InvestmentValue != 170000*3 || s.Currency != "MYR" || s.FXMissing {
		t.Errorf("past snapshot = %+v, want value %d (rate in effect then)", s, 170000*3)
	}

	later := t0.AddDate(0, 0, 15)
	_, err = svc.ComputeAndRecordSnapshots(later)
	testutil.AssertNoError(t, err)
	if s := snapshotOf(t, f.db, f.user.ID, later); s.InvestmentValue != 170000*5 {
		t.Errorf("later snapshot value = %d, want %d", s.InvestmentValue, 170000*5)
	}
}

func TestSnapshots_MissingRateSetsFXMissingAndUpsertUpdatesIt(t *testing.T) {
	f := newMCFixture(t) // no rate
	sec3 := testutil.CreateTestSecurityWithParams(t, f.db, "MYRS", "Myr", models.AssetTypeStock, "KLSE")
	inv3 := testutil.CreateTestInvestment(t, f.db, f.acct.ID, sec3.ID)
	testutil.AssertNoError(t, f.db.Model(inv3).Update("quantity", 2).Error)
	testutil.CreateTestSecurityPrice(t, f.db, sec3.ID, 500, time.Now())
	svc := NewPortfolioSnapshotService(f.db, f.fx)
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	_, err := svc.ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	s := snapshotOf(t, f.db, f.user.ID, at)
	if !s.FXMissing || s.InvestmentValue != 1000 || s.Currency != "MYR" {
		t.Errorf("snapshot = %+v, want fx_missing with only the MYR holding (1000)", s)
	}

	// The rate arrives; recomputing the same instant overwrites currency and fx_missing.
	f.seedUSDMYR(t, "4", at.Add(-time.Hour))
	_, err = svc.ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	s = snapshotOf(t, f.db, f.user.ID, at)
	if s.FXMissing || s.InvestmentValue != 1000+170000*4 {
		t.Errorf("recomputed snapshot = %+v", s)
	}
}

func TestSnapshots_PerUserBaseCurrency(t *testing.T) {
	f := newMCFixture(t)
	other := testutil.CreateTestUserWithEmail(t, f.db, "usd@example.com")
	testutil.AssertNoError(t, f.db.Model(other).Update("base_currency", "USD").Error)
	otherAcct := testutil.CreateTestInvestmentAccount(t, f.db, other.ID)
	testutil.CreateTestInvestment(t, f.db, otherAcct.ID, f.sec.ID)
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	f.seedUSDMYR(t, "4", at.Add(-time.Hour))

	_, err := NewPortfolioSnapshotService(f.db, f.fx).ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	if s := snapshotOf(t, f.db, f.user.ID, at); s.Currency != "MYR" || s.InvestmentValue != 680000 {
		t.Errorf("MYR user snapshot = %+v", s)
	}
	if s := snapshotOf(t, f.db, other.ID, at); s.Currency != "USD" || s.InvestmentValue != 170000 || s.FXMissing {
		t.Errorf("USD user snapshot = %+v", s)
	}
}

func TestSnapshots_UsesOneRatesCallPerBaseCurrency(t *testing.T) {
	f := newMCFixture(t)
	for _, email := range []string{"a@example.com", "b@example.com"} {
		u := testutil.CreateTestUserWithEmail(t, f.db, email)
		a := testutil.CreateTestInvestmentAccount(t, f.db, u.ID)
		testutil.CreateTestInvestment(t, f.db, a.ID, f.sec.ID)
	}
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	f.seedUSDMYR(t, "4", at.Add(-time.Hour))
	spy := &countingFX{FXServicer: f.fx}

	_, err := NewPortfolioSnapshotService(f.db, spy).ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	if spy.calls != 1 {
		t.Errorf("Rates calls = %d, want 1 (three MYR users)", spy.calls)
	}
}
