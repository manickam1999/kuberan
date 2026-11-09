package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/testutil"
)

// p4User creates a user with an explicit base currency (so the tests do not
// depend on the fixture default).
func p4User(t *testing.T, db *gorm.DB, base string) *models.User {
	t.Helper()
	u := testutil.CreateTestUser(t, db)
	testutil.AssertNoError(t, db.Model(u).Update("base_currency", base).Error)
	return u
}

// p4SetBase sets a user's base currency.
func p4SetBase(t *testing.T, db *gorm.DB, u *models.User, base string) {
	t.Helper()
	testutil.AssertNoError(t, db.Model(u).Update("base_currency", base).Error)
}

// p4AllAccountsIn moves every account to currency (older fixtures create USD
// accounts for MYR users; the snapshot tests want them in the base currency).
func p4AllAccountsIn(t *testing.T, db *gorm.DB, currency string) {
	t.Helper()
	testutil.AssertNoError(t, db.Model(&models.Account{}).Where("1 = 1").Update("currency", currency).Error)
}

// p4Account sets an account's currency.
func p4Account(t *testing.T, db *gorm.DB, a *models.Account, currency string) *models.Account {
	t.Helper()
	testutil.AssertNoError(t, db.Model(a).Update("currency", currency).Error)
	a.Currency = currency
	return a
}

// --- D. Accounts ---

func TestAccountsBaseBalance_FastPathNeedsNoFX(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "MYR")
	a := p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 12345), "MYR")
	svc := NewAccountService(db, WithFXService(panicFX{})) // any Rates/Rate call panics

	list, err := svc.GetUserAccounts(u.ID, pagination.PageRequest{Page: 1, PageSize: 10})
	testutil.AssertNoError(t, err)
	got := list.Data[0]
	if got.BalanceBase == nil || *got.BalanceBase != 12345 || got.FXRate != "" || got.FXMissing || got.FXStale {
		t.Errorf("list account = %+v base=%v", got, got.BalanceBase)
	}
	one, err := svc.GetAccountByID(u.ID, a.ID)
	testutil.AssertNoError(t, err)
	if one.BalanceBase == nil || *one.BalanceBase != 12345 {
		t.Errorf("get account balance_base = %v, want 12345", one.BalanceBase)
	}

	nilFX := NewAccountService(db) // nil FX service
	one, err = nilFX.GetAccountByID(u.ID, a.ID)
	testutil.AssertNoError(t, err)
	if one.BalanceBase == nil || *one.BalanceBase != 12345 {
		t.Errorf("nil fx: balance_base = %v, want 12345", one.BalanceBase)
	}
}

func TestAccountsBaseBalance_MixedMYRAndUSD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	fxs := NewFXService(db, testFXStaleness)
	u := p4User(t, db, "MYR")
	myr := p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 10000), "MYR")
	usd := p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 2500), "USD")
	seedRate(t, fxs, "USD", "MYR", "4.4712", time.Now().Add(-time.Hour))
	spy := &countingFX{FXServicer: fxs}
	svc := NewAccountService(db, WithFXService(spy))

	list, err := svc.GetUserAccounts(u.ID, pagination.PageRequest{Page: 1, PageSize: 10})
	testutil.AssertNoError(t, err)
	if spy.calls != 1 {
		t.Errorf("Rates calls = %d, want 1 batched lookup", spy.calls)
	}
	byID := map[string]models.Account{}
	for _, a := range list.Data {
		byID[a.ID] = a
	}
	// 2500 USD cents * 4.4712 = 11178.00 -> 11178 MYR cents.
	if b := byID[usd.ID]; b.BalanceBase == nil || *b.BalanceBase != 11178 || b.FXRate != "4.4712" || b.FXMissing {
		t.Errorf("USD account = base %v rate %q missing %v, want 11178 @ 4.4712", b.BalanceBase, b.FXRate, b.FXMissing)
	}
	if b := byID[myr.ID]; b.BalanceBase == nil || *b.BalanceBase != 10000 || b.FXRate != "" {
		t.Errorf("MYR account = base %v rate %q", b.BalanceBase, b.FXRate)
	}
	one, err := svc.GetAccountByID(u.ID, usd.ID)
	testutil.AssertNoError(t, err)
	if one.BalanceBase == nil || *one.BalanceBase != 11178 {
		t.Errorf("get USD balance_base = %v, want 11178", one.BalanceBase)
	}
}

func TestAccountsBaseBalance_MissingRateIsNotOneToOne(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "MYR")
	usd := p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 2500), "USD")
	svc := NewAccountService(db, WithFXService(NewFXService(db, testFXStaleness)))

	one, err := svc.GetAccountByID(u.ID, usd.ID)
	testutil.AssertNoError(t, err)
	if one.BalanceBase != nil || !one.FXMissing || one.FXRate != "" {
		t.Errorf("account = base %v missing %v rate %q, want no base and fx_missing", one.BalanceBase, one.FXMissing, one.FXRate)
	}
	if one.Balance != 2500 {
		t.Errorf("native balance changed to %d", one.Balance)
	}
}

// --- E. Snapshots ---

func TestSnapshots_FastPathNeedsNoFX(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "MYR")
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 100000), "MYR")
	p4Account(t, db, testutil.CreateTestCreditCardAccount(t, db, u.ID, 30000), "MYR")
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	for name, fxs := range map[string]FXServicer{"nil fx": nil, "panicking fx": panicFX{}} {
		t.Run(name, func(t *testing.T) {
			_, err := NewPortfolioSnapshotService(db, fxs).ComputeAndRecordSnapshots(at)
			testutil.AssertNoError(t, err)
			s := snapshotOf(t, db, u.ID, at)
			if s.CashBalance != 100000 || s.DebtBalance != 30000 || s.TotalNetWorth != 70000 || s.FXMissing || s.Currency != "MYR" {
				t.Errorf("snapshot = %+v", s)
			}
		})
	}
}

func TestSnapshots_USDCashAndCreditCardForMYRUserAtPastRecordedAt(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	fxs := NewFXService(db, testFXStaleness)
	u := p4User(t, db, "MYR")
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 100000), "MYR") // MYR
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 20000), "USD")  // USD cash
	p4Account(t, db, testutil.CreateTestCreditCardAccount(t, db, u.ID, 5000), "USD")        // USD card
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 7), "USD")      // second USD cash: grouped
	t0 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	seedRate(t, fxs, "USD", "MYR", "4", t0)
	seedRate(t, fxs, "USD", "MYR", "5", t0.AddDate(0, 0, 10))
	svc := NewPortfolioSnapshotService(db, fxs)

	past := t0.AddDate(0, 0, 5) // the rate of 4 is in effect, not the later 5
	_, err := svc.ComputeAndRecordSnapshots(past)
	testutil.AssertNoError(t, err)
	s := snapshotOf(t, db, u.ID, past)
	// Expected: MYR cash 100000 plus 20007 USD cents at 4 gives 180028; the USD card is 5000 at 4, so debt is 20000.
	if s.CashBalance != 180028 || s.DebtBalance != 20000 || s.TotalNetWorth != 160028 || s.FXMissing || s.Currency != "MYR" {
		t.Errorf("snapshot = %+v, want cash 180028 debt 20000 net 160028", s)
	}

	later := t0.AddDate(0, 0, 15)
	_, err = svc.ComputeAndRecordSnapshots(later)
	testutil.AssertNoError(t, err)
	if s := snapshotOf(t, db, u.ID, later); s.CashBalance != 100000+20007*5 || s.DebtBalance != 25000 {
		t.Errorf("later snapshot = %+v", s)
	}
}

func TestSnapshots_BalanceGroupWithoutRateIsSkippedAndFlagged(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	u := p4User(t, db, "MYR")
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 100000), "MYR") // MYR
	p4Account(t, db, testutil.CreateTestCashAccountWithBalance(t, db, u.ID, 20000), "USD")
	p4Account(t, db, testutil.CreateTestCreditCardAccount(t, db, u.ID, 5000), "USD")
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	_, err := NewPortfolioSnapshotService(db, NewFXService(db, testFXStaleness)).ComputeAndRecordSnapshots(at)
	testutil.AssertNoError(t, err)
	s := snapshotOf(t, db, u.ID, at)
	if !s.FXMissing || s.CashBalance != 100000 || s.DebtBalance != 0 || s.TotalNetWorth != 100000 {
		t.Errorf("snapshot = %+v, want only the MYR cash, fx_missing", s)
	}
}
